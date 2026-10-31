package filesystem

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
)

func TestArtifactSetSourceLoadsVerifiedISO(t *testing.T) {
	root, manifest, artifact := writeServingFixture(t, 1, "artifacts/agent.x86_64.iso", []byte("verified ISO"))
	set, err := (ArtifactSetSource{}).LoadAndVerify(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if set.Root != root || len(set.Artifacts) != 1 || set.Artifacts[0].FilePath != artifact {
		t.Fatalf("set=%#v", set)
	}
}

func TestArtifactSetSourceAcceptsCompletePXESet(t *testing.T) {
	manifest := writePXEServingFixture(t, []string{
		"artifacts/pxe/agent.x86_64-vmlinuz",
		"artifacts/pxe/agent.x86_64-initrd.img",
		"artifacts/pxe/agent.x86_64-rootfs.img",
		"artifacts/pxe/manifest.json",
	})
	set, err := (ArtifactSetSource{}).LoadAndVerify(manifest)
	if err != nil || len(set.Artifacts) != 4 {
		t.Fatalf("set=%#v error=%v", set, err)
	}
}

func TestArtifactSetSourceRejectsIncompleteAndDuplicatePXESets(t *testing.T) {
	complete := []string{
		"artifacts/pxe/agent.x86_64-vmlinuz",
		"artifacts/pxe/agent.x86_64-initrd.img",
		"artifacts/pxe/agent.x86_64-rootfs.img",
		"artifacts/pxe/manifest.json",
	}
	for _, test := range []struct {
		name  string
		paths []string
	}{
		{name: "incomplete", paths: complete[:3]},
		{name: "duplicate", paths: append(append([]string(nil), complete...), complete[0])},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := writePXEServingFixture(t, test.paths)
			if _, err := (ArtifactSetSource{}).LoadAndVerify(manifest); err == nil {
				t.Fatal("invalid PXE set was accepted")
			}
		})
	}
}

func TestArtifactSetSourceRejectsMissingModifiedAndUnsafeSets(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(t *testing.T, root, manifest, artifact string)
	}{
		{name: "missing", edit: func(t *testing.T, _, _, artifact string) { mustRemove(t, artifact) }},
		{name: "modified", edit: func(t *testing.T, _, _, artifact string) { mustWrite(t, artifact, []byte("modified")) }},
		{name: "unsupported version", edit: func(t *testing.T, _, manifest, _ string) {
			data, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			mustWrite(t, manifest, []byte(strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":2`, 1)))
		}},
		{name: "unsafe path", edit: func(t *testing.T, root, manifest, _ string) {
			digest := sha256.Sum256([]byte("outside"))
			writeManifest(t, manifest, 1, domain.Artifact{Path: "../outside", Size: 7, SHA256: fmt.Sprintf("sha256:%x", digest)})
			mustWrite(t, filepath.Join(root, "..", "outside"), []byte("outside"))
		}},
		{name: "symlink", edit: func(t *testing.T, root, _, artifact string) {
			mustRemove(t, artifact)
			outside := filepath.Join(root, "outside")
			mustWrite(t, outside, []byte("verified ISO"))
			if err := os.Symlink(outside, artifact); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, manifest, artifact := writeServingFixture(t, 1, "artifacts/agent.x86_64.iso", []byte("verified ISO"))
			test.edit(t, root, manifest, artifact)
			if _, err := (ArtifactSetSource{}).LoadAndVerify(manifest); err == nil {
				t.Fatal("unsafe artifact set was accepted")
			}
		})
	}
}

func writeServingFixture(t *testing.T, version int, path string, content []byte) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	artifact := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, artifact, content)
	digest := sha256.Sum256(content)
	manifest := filepath.Join(root, "run.json")
	writeManifest(t, manifest, version, domain.Artifact{Path: path, Size: int64(len(content)), SHA256: fmt.Sprintf("sha256:%x", digest)})
	return root, manifest, artifact
}

func writePXEServingFixture(t *testing.T, paths []string) string {
	t.Helper()
	root := t.TempDir()
	artifacts := make([]domain.Artifact, 0, len(paths))
	for index, path := range paths {
		content := []byte(fmt.Sprintf("artifact-%d", index))
		artifactPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
			t.Fatal(err)
		}
		// A duplicate manifest entry refers to the original file and checksum.
		if _, err := os.Stat(artifactPath); errors.Is(err, os.ErrNotExist) {
			mustWrite(t, artifactPath, content)
		} else if err == nil {
			content, err = os.ReadFile(artifactPath)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		artifacts = append(artifacts, domain.Artifact{Path: path, Size: int64(len(content)), SHA256: fmt.Sprintf("sha256:%x", digest)})
	}
	manifest := filepath.Join(root, "run.json")
	writeManifest(t, manifest, 1, artifacts...)
	return manifest
}

func writeManifest(t *testing.T, path string, version int, artifacts ...domain.Artifact) {
	t.Helper()
	data, err := json.Marshal(struct {
		SchemaVersion int               `json:"schemaVersion"`
		Outcome       string            `json:"outcome"`
		Artifacts     []domain.Artifact `json:"artifacts"`
	}{version, "success", artifacts})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, data)
}

func mustWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
