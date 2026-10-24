package coreos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mozebaltyk/Okub/internal/domain"
)

const maxMetadataSize = 4 << 20

var (
	repositoryPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]+)?/[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)
	releaseTagPattern = regexp.MustCompile(`^4\.\d+\.\d+[A-Za-z0-9._-]*$`)
)

// Resolver combines the OKD release-controller stable stream with the matching
// immutable GitHub release assets.
type Resolver struct {
	ReleaseControllerURL string
	GitHubAPIURL         string
	Client               *http.Client
	AllowHTTP            bool // tests using an httptest server only
	Timeout              time.Duration
}

func (r Resolver) Resolve(ctx context.Context, requestedVersion, architecture string) (domain.Release, error) {
	if architecture != "x86_64" {
		return domain.Release{}, errors.New("release architecture is unsupported")
	}
	var stream struct {
		Tags []struct {
			Name     string `json:"name"`
			Phase    string `json:"phase"`
			PullSpec string `json:"pullSpec"`
		} `json:"tags"`
	}
	if err := r.getJSON(ctx, r.ReleaseControllerURL, &stream); err != nil {
		return domain.Release{}, err
	}
	tags := stream.Tags[:0]
	for _, tag := range stream.Tags {
		if tag.Phase == "Accepted" && releaseTagPattern.MatchString(tag.Name) && versionMatches(requestedVersion, tag.Name) && validTaggedPullSpec(tag.PullSpec, tag.Name) {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return domain.Release{}, errors.New("requested accepted OKD release is unavailable")
	}
	sort.Slice(tags, func(i, j int) bool { return naturalLess(tags[j].Name, tags[i].Name) })
	selected := tags[0]
	detailURL, err := releaseDetailURL(r.ReleaseControllerURL, selected.Name)
	if err != nil {
		return domain.Release{}, err
	}
	var detail struct {
		ChangeLogJSON struct {
			To struct {
				Name   string `json:"name"`
				Digest string `json:"digest"`
			} `json:"to"`
		} `json:"changeLogJson"`
	}
	if err := r.getJSON(ctx, detailURL, &detail); err != nil {
		return domain.Release{}, err
	}
	if detail.ChangeLogJSON.To.Name != selected.Name || !validSHA256(detail.ChangeLogJSON.To.Digest) {
		return domain.Release{}, errors.New("release detail does not match the selected tag or immutable digest")
	}
	imagePullSpec, err := immutablePullSpec(selected.PullSpec, selected.Name, detail.ChangeLogJSON.To.Digest)
	if err != nil {
		return domain.Release{}, err
	}

	githubURL, err := url.Parse(r.GitHubAPIURL)
	if err != nil || githubURL.Host == "" {
		return domain.Release{}, errors.New("GitHub release endpoint is invalid")
	}
	githubURL.Path = path.Join(githubURL.Path, "repos/okd-project/okd/releases/tags", selected.Name)
	var githubRelease struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Digest             string `json:"digest"`
		} `json:"assets"`
	}
	if err := r.getJSON(ctx, githubURL.String(), &githubRelease); err != nil {
		return domain.Release{}, err
	}
	if githubRelease.TagName != selected.Name {
		return domain.Release{}, errors.New("GitHub release tag does not match the selected OKD release")
	}
	installerName := "openshift-install-linux-" + selected.Name + ".tar.gz"
	clientName := "openshift-client-linux-" + selected.Name + ".tar.gz"
	wanted := map[string]string{installerName: "installer", clientName: "client", "release.txt": "release-metadata"}
	release := domain.Release{
		Version: selected.Name, Architecture: architecture, InstallerVersion: selected.Name,
		ImagePullSpec: imagePullSpec, ImageDigest: detail.ChangeLogJSON.To.Digest,
	}
	for _, asset := range githubRelease.Assets {
		name, found := wanted[asset.Name]
		if !found {
			continue
		}
		if !validSHA256(asset.Digest) || asset.Size <= 0 {
			return domain.Release{}, errors.New("GitHub release asset is missing a valid digest or size")
		}
		release.Inputs = append(release.Inputs, domain.ReleaseInput{
			Name: name, URL: asset.BrowserDownloadURL, Digest: asset.Digest, Size: asset.Size,
		})
	}
	if len(release.Inputs) != len(wanted) {
		return domain.Release{}, errors.New("GitHub release is missing required checksum-pinned assets")
	}
	sort.Slice(release.Inputs, func(i, j int) bool { return release.Inputs[i].Name < release.Inputs[j].Name })
	return release, nil
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}

func (r Resolver) getJSON(ctx context.Context, rawURL string, target any) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && !(r.AllowHTTP && parsed.Scheme == "http") {
		return errors.New("release metadata endpoint is invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return errors.New("create release metadata request")
	}
	client := r.Client
	if client == nil {
		timeout := r.Timeout
		if timeout == 0 {
			timeout = 15 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	response, err := securedClient(client, r.AllowHTTP).Do(request)
	if err != nil {
		return errors.New("release metadata is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("release metadata returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetadataSize+1))
	if err != nil || len(data) > maxMetadataSize {
		return errors.New("release metadata cannot be read")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(target); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("release metadata is invalid")
	}
	return nil
}

func versionMatches(requested, exact string) bool {
	parts := strings.Split(requested, ".")
	if len(parts) == 2 {
		return strings.HasPrefix(exact, requested+".")
	}
	return exact == requested
}

func releaseDetailURL(tagsURL, tag string) (string, error) {
	parsed, err := url.Parse(tagsURL)
	if err != nil || !strings.HasSuffix(strings.TrimSuffix(parsed.Path, "/"), "/tags") {
		return "", errors.New("release-controller tags endpoint is invalid")
	}
	parsed.Path = path.Join(path.Dir(strings.TrimSuffix(parsed.Path, "/")), "release", tag)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func validTaggedPullSpec(value, tag string) bool {
	_, err := immutablePullSpec(value, tag, "sha256:"+strings.Repeat("0", 64))
	return err == nil
}

func immutablePullSpec(value, tag, digest string) (string, error) {
	if strings.ContainsAny(value, "@?# \\") || !validSHA256(digest) {
		return "", errors.New("release tag pullspec or digest is invalid")
	}
	separator := strings.LastIndex(value, ":")
	if separator <= strings.LastIndex(value, "/") || value[separator+1:] != tag {
		return "", errors.New("release tag pullspec does not match the selected tag")
	}
	repository := value[:separator]
	if !repositoryPattern.MatchString(repository) {
		return "", errors.New("release image registry or repository is invalid")
	}
	return repository + "@" + strings.ToLower(digest), nil
}

func naturalLess(left, right string) bool {
	for len(left) > 0 && len(right) > 0 {
		leftDigits, rightDigits := unicode.IsDigit(rune(left[0])), unicode.IsDigit(rune(right[0]))
		leftEnd, rightEnd := 1, 1
		for leftEnd < len(left) && unicode.IsDigit(rune(left[leftEnd])) == leftDigits {
			leftEnd++
		}
		for rightEnd < len(right) && unicode.IsDigit(rune(right[rightEnd])) == rightDigits {
			rightEnd++
		}
		leftPart, rightPart := left[:leftEnd], right[:rightEnd]
		if leftDigits {
			leftPart, rightPart = strings.TrimLeft(leftPart, "0"), strings.TrimLeft(rightPart, "0")
			if len(leftPart) != len(rightPart) {
				return len(leftPart) < len(rightPart)
			}
		}
		if leftPart != rightPart {
			return leftPart < rightPart
		}
		left, right = left[leftEnd:], right[rightEnd:]
	}
	return len(left) < len(right)
}

// HTTPDownloader retrieves checksum-pinned inputs without buffering them.
type HTTPDownloader struct {
	Client    *http.Client
	AllowHTTP bool // tests using an httptest server only
	Timeout   time.Duration
}

func (d HTTPDownloader) Open(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && !(d.AllowHTTP && parsed.Scheme == "http") {
		return nil, errors.New("artifact URL is invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, errors.New("create artifact request")
	}
	client := d.Client
	if client == nil {
		timeout := d.Timeout
		if timeout == 0 {
			timeout = 10 * time.Minute
		}
		client = &http.Client{Timeout: timeout}
	}
	response, err := securedClient(client, d.AllowHTTP).Do(request)
	if err != nil {
		return nil, errors.New("artifact download failed")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("artifact endpoint returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

func securedClient(client *http.Client, allowHTTP bool) *http.Client {
	secured := *client
	previous := client.CheckRedirect
	secured.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.User != nil || request.URL.Scheme != "https" && !(allowHTTP && request.URL.Scheme == "http") {
			return errors.New("redirect target is not permitted")
		}
		if previous != nil {
			return previous(request, via)
		}
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}
	return &secured
}
