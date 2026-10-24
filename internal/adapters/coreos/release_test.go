package coreos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolverUsesProductionContractsAndSelectsNewestAcceptedTag(t *testing.T) {
	const selected = "4.20.0-okd-scos.10"
	var detailPath, githubPath string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/controller/tags":
			json.NewEncoder(response).Encode(map[string]any{"tags": []map[string]string{
				{"name": "4.20.0-okd-scos.99", "phase": "Rejected", "pullSpec": taggedPullSpec("4.20.0-okd-scos.99")},
				{"name": "4.20.0-okd-scos.9", "phase": "Accepted", "pullSpec": taggedPullSpec("4.20.0-okd-scos.9")},
				{"name": "4.21.0-okd-scos.1", "phase": "Accepted", "pullSpec": taggedPullSpec("4.21.0-okd-scos.1")},
				{"name": selected, "phase": "Accepted", "pullSpec": taggedPullSpec(selected)},
			}})
		case request.URL.Path == "/controller/release/"+selected:
			detailPath = request.URL.Path
			json.NewEncoder(response).Encode(releaseDetail(selected, digest("3")))
		default:
			githubPath = request.URL.Path
			json.NewEncoder(response).Encode(githubRelease(server.URL, selected, digest("a"), false))
		}
	}))
	defer server.Close()

	release, err := (Resolver{ReleaseControllerURL: server.URL + "/controller/tags", GitHubAPIURL: server.URL + "/github", AllowHTTP: true}).Resolve(context.Background(), "4.20", "x86_64")
	if err != nil || release.Version != selected || release.ImageDigest != digest("3") || release.ImagePullSpec != "quay.io/okd/scos-release@"+digest("3") || len(release.Inputs) != 3 {
		t.Fatalf("release=%#v error=%v", release, err)
	}
	if detailPath != "/controller/release/"+selected {
		t.Fatalf("detail path=%q", detailPath)
	}
	if githubPath != "/github/repos/okd-project/okd/releases/tags/"+selected {
		t.Fatalf("GitHub path=%q", githubPath)
	}
}

func TestResolverRejectsInvalidProductionMetadata(t *testing.T) {
	const tag = "4.20.1-okd-scos.1"
	tests := []struct {
		name, pullSpec, detailName, detailDigest, assetDigest string
		omitAsset                                             bool
	}{
		{name: "malformed pullspec", pullSpec: "https://user:secret@quay.io/okd:" + tag, detailName: tag, detailDigest: digest("1"), assetDigest: digest("a")},
		{name: "mismatched detail tag", pullSpec: taggedPullSpec(tag), detailName: "4.20.2-other", detailDigest: digest("1"), assetDigest: digest("a")},
		{name: "invalid detail digest", pullSpec: taggedPullSpec(tag), detailName: tag, detailDigest: "sha256:short", assetDigest: digest("a")},
		{name: "missing asset digest", pullSpec: taggedPullSpec(tag), detailName: tag, detailDigest: digest("1")},
		{name: "missing release asset", pullSpec: taggedPullSpec(tag), detailName: tag, detailDigest: digest("1"), assetDigest: digest("a"), omitAsset: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				switch {
				case strings.HasSuffix(request.URL.Path, "/tags"):
					json.NewEncoder(response).Encode(map[string]any{"tags": []map[string]string{{"name": tag, "phase": "Accepted", "pullSpec": test.pullSpec}}})
				case strings.Contains(request.URL.Path, "/release/"):
					json.NewEncoder(response).Encode(releaseDetail(test.detailName, test.detailDigest))
				default:
					json.NewEncoder(response).Encode(githubRelease(server.URL, tag, test.assetDigest, test.omitAsset))
				}
			}))
			defer server.Close()
			_, err := (Resolver{ReleaseControllerURL: server.URL + "/tags", GitHubAPIURL: server.URL, AllowHTTP: true}).Resolve(context.Background(), "4.20", "x86_64")
			if err == nil {
				t.Fatal("Resolve succeeded")
			}
		})
	}
}

func TestDownloaderRejectsInsecureRedirect(t *testing.T) {
	insecure := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, insecure.URL, http.StatusFound)
	}))
	defer secure.Close()
	_, err := (HTTPDownloader{Client: secure.Client()}).Open(context.Background(), secure.URL)
	if err == nil {
		t.Fatal("insecure redirect was accepted")
	}
}

func TestResolverRejectsInsecureRedirect(t *testing.T) {
	insecure := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, insecure.URL, http.StatusFound)
	}))
	defer secure.Close()
	_, err := (Resolver{ReleaseControllerURL: secure.URL + "/tags", GitHubAPIURL: secure.URL, Client: secure.Client()}).Resolve(context.Background(), "4.20", "x86_64")
	if err == nil {
		t.Fatal("insecure metadata redirect was accepted")
	}
}

func taggedPullSpec(tag string) string { return "quay.io/okd/scos-release:" + tag }

func releaseDetail(name, value string) map[string]any {
	return map[string]any{"changeLogJson": map[string]any{"to": map[string]string{"name": name, "digest": value}}}
}

func githubRelease(baseURL, tag, assetDigest string, omitRelease bool) map[string]any {
	assets := []map[string]any{{"name": "openshift-install-linux-" + tag + ".tar.gz", "size": 34, "browser_download_url": baseURL + "/installer", "digest": assetDigest}, {"name": "openshift-client-linux-" + tag + ".tar.gz", "size": 30, "browser_download_url": baseURL + "/client", "digest": assetDigest}}
	if !omitRelease {
		assets = append(assets, map[string]any{"name": "release.txt", "size": 12, "browser_download_url": baseURL + "/release.txt", "digest": assetDigest})
	}
	return map[string]any{"tag_name": tag, "assets": assets}
}

func digest(character string) string {
	return fmt.Sprintf("sha256:%s", strings.Repeat(character, 64))
}
