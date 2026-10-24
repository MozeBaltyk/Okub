package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	cacheadapter "github.com/mozebaltyk/Okub/internal/adapters/cache"
	"github.com/mozebaltyk/Okub/internal/adapters/coreos"
	"github.com/mozebaltyk/Okub/internal/domain"
)

func TestImageSyncCacheHitAndStrictOffline(t *testing.T) {
	root := t.TempDir()
	server, metadataCalls, artifactCalls := releaseFixtureServer(t, "4.20.1-okd", false)
	defer server.Close()
	workflow := fixtureImageSync(root, server.URL)

	first, report, err := workflow.Sync(context.Background(), "ignored", false)
	if err != nil || !report.Valid || first.Cached || first.ImageDigest == "" || *metadataCalls != 3 || *artifactCalls != 3 {
		t.Fatalf("first=%#v report=%#v error=%v metadata=%d artifacts=%d", first, report, err, *metadataCalls, *artifactCalls)
	}
	second, _, err := workflow.Sync(context.Background(), "ignored", false)
	if err != nil || !second.Cached || second.ReleaseVersion != first.ReleaseVersion || len(second.Inputs) != len(first.Inputs) || *metadataCalls != 3 || *artifactCalls != 3 {
		t.Fatalf("second=%#v error=%v metadata=%d artifacts=%d", second, err, *metadataCalls, *artifactCalls)
	}
	offline, _, err := workflow.Sync(context.Background(), "ignored", true)
	if err != nil || !offline.Cached || *metadataCalls != 3 || *artifactCalls != 3 {
		t.Fatalf("offline=%#v error=%v metadata=%d artifacts=%d", offline, err, *metadataCalls, *artifactCalls)
	}

	miss := fixtureImageSync(t.TempDir(), server.URL)
	_, _, err = miss.Sync(context.Background(), "ignored", true)
	var cacheErr *CacheError
	if !errors.As(err, &cacheErr) || *metadataCalls != 3 || *artifactCalls != 3 {
		t.Fatalf("offline miss error=%v metadata=%d artifacts=%d", err, *metadataCalls, *artifactCalls)
	}
}

func TestCacheErrorIncludesAndWrapsCause(t *testing.T) {
	cause := errors.New("archive does not contain oc")
	err := &CacheError{Err: cause}
	if !strings.Contains(err.Error(), "verified cache operation failed") || !strings.Contains(err.Error(), cause.Error()) || !errors.Is(err, cause) {
		t.Fatalf("error=%q wraps=%t", err, errors.Is(err, cause))
	}
}

func TestImageSyncResolverUnavailableAndMissingAsset(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()
		_, _, err := fixtureImageSync(t.TempDir(), server.URL).Sync(context.Background(), "ignored", false)
		var networkErr *NetworkError
		if !errors.As(err, &networkErr) || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("missing required asset", func(t *testing.T) {
		server, _, _ := releaseFixtureServer(t, "4.20.1-okd", true)
		defer server.Close()
		_, _, err := fixtureImageSync(t.TempDir(), server.URL).Sync(context.Background(), "ignored", false)
		var networkErr *NetworkError
		if !errors.As(err, &networkErr) {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestImageSyncRejectsIncompatibleInstaller(t *testing.T) {
	err := validateResolvedRelease(domain.Release{
		Version: "4.20.1-okd", Architecture: "x86_64", InstallerVersion: "4.19.9-okd",
	}, "4.20", "x86_64")
	var compatibilityErr *CompatibilityError
	if !errors.As(err, &compatibilityErr) {
		t.Fatalf("error=%v", err)
	}
}

func TestImageSyncConcurrentPublication(t *testing.T) {
	root := t.TempDir()
	server, _, _ := releaseFixtureServer(t, "4.20.1-okd", false)
	defer server.Close()
	workflow := fixtureImageSync(root, server.URL)
	var wait sync.WaitGroup
	errorsFound := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, _, err := workflow.Sync(context.Background(), "ignored", false); err != nil {
				errorsFound <- err
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	if _, found, err := (cacheadapter.Release{}).Lookup(root, "4.20", "x86_64"); err != nil || !found {
		t.Fatalf("found=%t error=%v", found, err)
	}
}

func fixtureImageSync(cacheRoot, baseURL string) ImageSync {
	return ImageSync{
		Validator: ConfigurationValidator{Source: sourceStub{data: []byte(preflightConfig)}, CacheDirectory: cacheRoot},
		Resolver: coreos.Resolver{
			ReleaseControllerURL: baseURL + "/release-controller/tags", GitHubAPIURL: baseURL + "/github", AllowHTTP: true,
		},
		Downloader: coreos.HTTPDownloader{AllowHTTP: true}, Cache: cacheadapter.Release{},
	}
}

func releaseFixtureServer(t *testing.T, releaseVersion string, omitReleaseMetadata bool) (*httptest.Server, *int32, *int32) {
	t.Helper()
	contents := map[string][]byte{"installer": []byte("installer-fixture"), "client": []byte("client-fixture"), "release-metadata": []byte("release metadata fixture")}
	var metadataCalls, artifactCalls int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/release-controller/tags":
			atomic.AddInt32(&metadataCalls, 1)
			stream := map[string]any{"tags": []map[string]string{
				{"name": "4.20.99-rejected", "phase": "Rejected", "pullSpec": "quay.io/okd/scos-release:4.20.99-rejected"},
				{"name": releaseVersion, "phase": "Accepted", "pullSpec": "quay.io/okd/scos-release:" + releaseVersion},
			}}
			if err := json.NewEncoder(response).Encode(stream); err != nil {
				t.Error(err)
			}
			return
		case "/release-controller/release/" + releaseVersion:
			atomic.AddInt32(&metadataCalls, 1)
			detail := map[string]any{"changeLogJson": map[string]any{"to": map[string]string{
				"name": releaseVersion, "digest": fmt.Sprintf("sha256:%064d", 2),
			}}}
			if err := json.NewEncoder(response).Encode(detail); err != nil {
				t.Error(err)
			}
			return
		case "/github/repos/okd-project/okd/releases/tags/" + releaseVersion:
			atomic.AddInt32(&metadataCalls, 1)
			type asset struct {
				Name   string `json:"name"`
				Size   int64  `json:"size"`
				URL    string `json:"browser_download_url"`
				Digest string `json:"digest"`
			}
			release := struct {
				TagName string  `json:"tag_name"`
				Assets  []asset `json:"assets"`
			}{TagName: releaseVersion}
			for _, name := range []string{"release-metadata", "installer", "client"} {
				if name == "release-metadata" && omitReleaseMetadata {
					continue
				}
				digest := sha256.Sum256(contents[name])
				assetName := "release.txt"
				if name == "installer" {
					assetName = "openshift-install-linux-" + releaseVersion + ".tar.gz"
				} else if name == "client" {
					assetName = "openshift-client-linux-" + releaseVersion + ".tar.gz"
				}
				release.Assets = append(release.Assets, asset{
					Name: assetName, URL: server.URL + "/artifacts/" + name,
					Digest: fmt.Sprintf("sha256:%x", digest), Size: int64(len(contents[name])),
				})
			}
			if err := json.NewEncoder(response).Encode(release); err != nil {
				t.Error(err)
			}
			return
		}
		if strings.HasPrefix(request.URL.Path, "/artifacts/") {
			atomic.AddInt32(&artifactCalls, 1)
			response.Write(contents[strings.TrimPrefix(request.URL.Path, "/artifacts/")])
			return
		}
		http.NotFound(response, request)
	}))
	return server, &metadataCalls, &artifactCalls
}
