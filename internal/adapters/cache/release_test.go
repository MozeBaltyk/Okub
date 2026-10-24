package cache

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
)

func TestReleaseCachePublishesAndRejectsCorruption(t *testing.T) {
	root := t.TempDir()
	release, contents := fixtureRelease()
	cache := Release{}
	for _, input := range release.Inputs {
		if err := cache.StoreBlob(root, input, bytes.NewReader(contents[input.Name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := cache.Publish(root, release); err != nil {
		t.Fatal(err)
	}
	manifestFile, _ := manifestPath(root, "4.20", "x86_64")
	firstManifest, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Publish(root, release); err != nil {
		t.Fatal(err)
	}
	secondManifest, err := os.ReadFile(manifestFile)
	if err != nil || !bytes.Equal(firstManifest, secondManifest) {
		t.Fatalf("manifest changed: error=%v", err)
	}
	got, found, err := cache.Lookup(root, "4.20", "x86_64")
	if err != nil || !found || got.Version != release.Version || got.ImagePullSpec != release.ImagePullSpec || got.ImageDigest != release.ImageDigest || len(got.Inputs) != 3 {
		t.Fatalf("got=%#v found=%t error=%v", got, found, err)
	}
	blob, _ := blobPath(root, release.Inputs[0].Digest)
	if err := os.WriteFile(blob, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cache.Lookup(root, "4.20", "x86_64"); err != nil || found {
		t.Fatalf("corrupt found=%t error=%v", found, err)
	}
}

func TestReleaseCacheNeverPublishesTruncatedOrMismatchedBlob(t *testing.T) {
	for _, test := range []struct {
		name    string
		content []byte
	}{
		{"truncated", []byte("installer")},
		{"mismatched", []byte("same-size-wrong!!")},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			release, _ := fixtureRelease()
			input := release.Inputs[0]
			if err := (Release{}).StoreBlob(root, input, bytes.NewReader(test.content)); err == nil {
				t.Fatal("StoreBlob succeeded")
			}
			path, _ := blobPath(root, input.Digest)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("invalid blob was published: %v", err)
			}
		})
	}
}

func TestReleaseCacheConcurrentInsertion(t *testing.T) {
	root := t.TempDir()
	release, contents := fixtureRelease()
	cache := Release{}
	var wait sync.WaitGroup
	errorsFound := make(chan error, 32)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for _, input := range release.Inputs {
				if err := cache.StoreBlob(root, input, bytes.NewReader(contents[input.Name])); err != nil {
					errorsFound <- err
					return
				}
			}
			if err := cache.Publish(root, release); err != nil {
				errorsFound <- err
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	if _, found, err := cache.Lookup(root, "4.20", "x86_64"); err != nil || !found {
		t.Fatalf("found=%t error=%v", found, err)
	}
	stages, err := filepath.Glob(filepath.Join(root, "**", ".download-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("staging files=%v error=%v", stages, err)
	}
}

func fixtureRelease() (domain.Release, map[string][]byte) {
	contents := map[string][]byte{
		"installer":        []byte("installer-binary"),
		"client":           []byte("client-binary"),
		"release-metadata": []byte("release-metadata"),
	}
	inputs := make([]domain.ReleaseInput, 0, 3)
	for _, name := range []string{"installer", "client", "release-metadata"} {
		digest := sha256.Sum256(contents[name])
		inputs = append(inputs, domain.ReleaseInput{Name: name, Digest: fmt.Sprintf("sha256:%x", digest), Size: int64(len(contents[name]))})
	}
	return domain.Release{
		RequestedVersion: "4.20", Version: "4.20.1-okd", Architecture: "x86_64",
		InstallerVersion: "4.20.1-okd", ImagePullSpec: "quay.io/okd/release@sha256:" + strings.Repeat("a", 64),
		ImageDigest: "sha256:" + strings.Repeat("a", 64), Inputs: inputs,
	}, contents
}
