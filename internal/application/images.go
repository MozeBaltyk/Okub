package application

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

// ImageSync caches HTTP release inputs and pins the immutable OCI image reference.
type ImageSync struct {
	Validator      ConfigurationValidator
	Resolver       ports.ReleaseResolver
	Downloader     ports.Downloader
	Cache          ports.ReleaseCache
	Extractor      ports.ToolExtractor
	ToolsDirectory string
}

// ImageSyncResult is the stable, path-free result of images sync.
type ImageSyncResult struct {
	ReleaseVersion   string                 `json:"releaseVersion"`
	Architecture     string                 `json:"architecture"`
	InstallerVersion string                 `json:"installerVersion"`
	ImagePullSpec    string                 `json:"imagePullSpec"`
	ImageDigest      string                 `json:"imageDigest"`
	Cached           bool                   `json:"cached"`
	Inputs           []ImageSyncInputResult `json:"inputs"`
}

type ImageSyncInputResult struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	Cached bool   `json:"cached"`
}

type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "release or download operation failed" }
func (e *NetworkError) Unwrap() error { return e.Err }

type CacheError struct{ Err error }

func (e *CacheError) Error() string {
	if e.Err == nil {
		return "verified cache operation failed"
	}
	return fmt.Sprintf("verified cache operation failed: %v", e.Err)
}
func (e *CacheError) Unwrap() error { return e.Err }

type CompatibilityError struct{ Message string }

func (e *CompatibilityError) Error() string { return e.Message }

func (s ImageSync) Sync(ctx context.Context, configPath string, offline bool) (ImageSyncResult, domain.ValidationReport, error) {
	validated, err := s.Validator.Validate(configPath)
	if err != nil || !validated.Report.Valid {
		return ImageSyncResult{}, validated.Report, err
	}
	c := validated.Definition
	release, found, err := s.Cache.Lookup(c.Artifacts.CacheDirectory, c.Cluster.Version, c.Cluster.Architecture)
	if err != nil {
		return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
	}
	if found {
		if err := validateCachedRelease(release, c.Cluster.Version, c.Cluster.Architecture); err != nil {
			return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
		}
		if err := s.materializeTools(c.Artifacts.CacheDirectory, release); err != nil {
			return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
		}
		return imageSyncResult(release, nil), validated.Report, nil
	}
	if offline {
		return ImageSyncResult{}, validated.Report, &CacheError{Err: errors.New("requested release is not available in the verified cache")}
	}
	release, err = s.Resolver.Resolve(ctx, c.Cluster.Version, c.Cluster.Architecture)
	if err != nil {
		return ImageSyncResult{}, validated.Report, &NetworkError{Err: err}
	}
	release.RequestedVersion = c.Cluster.Version
	if err := validateResolvedRelease(release, c.Cluster.Version, c.Cluster.Architecture); err != nil {
		return ImageSyncResult{}, validated.Report, err
	}
	sort.Slice(release.Inputs, func(i, j int) bool { return release.Inputs[i].Name < release.Inputs[j].Name })
	downloaded := map[string]bool{}
	for _, input := range release.Inputs {
		cached, err := s.Cache.HasBlob(c.Artifacts.CacheDirectory, input)
		if err != nil {
			return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
		}
		if cached {
			continue
		}
		reader, err := s.Downloader.Open(ctx, input.URL)
		if err != nil {
			return ImageSyncResult{}, validated.Report, &NetworkError{Err: err}
		}
		storeErr := s.Cache.StoreBlob(c.Artifacts.CacheDirectory, input, reader)
		closeErr := reader.Close()
		if storeErr != nil {
			if errors.Is(storeErr, ports.ErrIntegrity) {
				return ImageSyncResult{}, validated.Report, &NetworkError{Err: storeErr}
			}
			return ImageSyncResult{}, validated.Report, &CacheError{Err: storeErr}
		}
		if closeErr != nil {
			return ImageSyncResult{}, validated.Report, &NetworkError{Err: closeErr}
		}
		downloaded[input.Name] = true
	}
	if err := s.Cache.Publish(c.Artifacts.CacheDirectory, release); err != nil {
		return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
	}
	verified, found, err := s.Cache.Lookup(c.Artifacts.CacheDirectory, c.Cluster.Version, c.Cluster.Architecture)
	if err != nil || !found {
		if err == nil {
			err = errors.New("published release did not verify")
		}
		return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
	}
	if err := s.materializeTools(c.Artifacts.CacheDirectory, verified); err != nil {
		return ImageSyncResult{}, validated.Report, &CacheError{Err: err}
	}
	return imageSyncResult(verified, downloaded), validated.Report, nil
}

func (s ImageSync) materializeTools(cacheRoot string, release domain.Release) error {
	if s.Extractor == nil || s.ToolsDirectory == "" {
		return nil
	}
	directory := filepath.Join(s.ToolsDirectory, release.Version)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create tool directory: %w", err)
	}
	for _, item := range []struct{ input, executable string }{{"installer", "openshift-install"}, {"client", "oc"}} {
		input, found := releaseInput(release, item.input)
		if !found {
			return fmt.Errorf("cached %s input is missing", item.input)
		}
		archive, err := s.Cache.OpenBlob(cacheRoot, input)
		if err != nil {
			return err
		}
		stage, err := os.CreateTemp(directory, "."+item.executable+"-")
		if err != nil {
			archive.Close()
			return err
		}
		stagePath := stage.Name()
		stage.Close()
		os.Remove(stagePath)
		extractErr := s.Extractor.ExtractExecutable(archive, stagePath, item.executable)
		closeErr := archive.Close()
		if extractErr != nil || closeErr != nil {
			os.Remove(stagePath)
			return errors.Join(extractErr, closeErr)
		}
		if err := os.Rename(stagePath, filepath.Join(directory, item.executable)); err != nil {
			os.Remove(stagePath)
			return err
		}
	}
	return nil
}

var releaseVersionPattern = regexp.MustCompile(`^4\.\d+(?:\.\d+(?:[-+.][A-Za-z0-9.-]+)?)?$`)

func validateResolvedRelease(release domain.Release, requestedVersion, architecture string) error {
	if err := validateReleaseIdentity(release, requestedVersion, architecture); err != nil {
		return err
	}
	for _, input := range release.Inputs {
		parsed, err := url.Parse(input.URL)
		if err != nil || parsed.Host == "" || parsed.User != nil {
			return &CompatibilityError{Message: "resolved release input metadata is invalid"}
		}
	}
	return nil
}

func validateCachedRelease(release domain.Release, requestedVersion, architecture string) error {
	return validateReleaseIdentity(release, requestedVersion, architecture)
}

func validateReleaseIdentity(release domain.Release, requestedVersion, architecture string) error {
	if !releaseVersionPattern.MatchString(release.Version) || release.Architecture != architecture || releaseSeries(release.Version) != releaseSeries(requestedVersion) || releaseSeries(release.InstallerVersion) != releaseSeries(release.Version) {
		return &CompatibilityError{Message: "resolved release or installer is incompatible with the request"}
	}
	if len(release.Inputs) != 3 {
		return &CompatibilityError{Message: "resolved release does not contain exactly the required inputs"}
	}
	imageDigest := strings.TrimPrefix(strings.ToLower(release.ImageDigest), "sha256:")
	if !strings.HasSuffix(release.ImagePullSpec, "@sha256:"+imageDigest) || len(imageDigest) != 64 {
		return &CompatibilityError{Message: "resolved release image reference is not immutable"}
	}
	if _, err := hex.DecodeString(imageDigest); err != nil {
		return &CompatibilityError{Message: "resolved release image reference is not immutable"}
	}
	wanted := map[string]bool{"installer": false, "client": false, "release-metadata": false}
	for _, input := range release.Inputs {
		digest := strings.TrimPrefix(strings.ToLower(input.Digest), "sha256:")
		_, digestErr := hex.DecodeString(digest)
		if _, exists := wanted[input.Name]; !exists || wanted[input.Name] || digestErr != nil || len(digest) != 64 || input.Size <= 0 {
			return &CompatibilityError{Message: "resolved release input metadata is invalid"}
		}
		wanted[input.Name] = true
	}
	return nil
}

func imageSyncResult(release domain.Release, downloaded map[string]bool) ImageSyncResult {
	result := ImageSyncResult{
		ReleaseVersion: release.Version, Architecture: release.Architecture,
		InstallerVersion: release.InstallerVersion, ImagePullSpec: release.ImagePullSpec,
		ImageDigest: release.ImageDigest, Cached: len(downloaded) == 0,
	}
	for _, input := range release.Inputs {
		result.Inputs = append(result.Inputs, ImageSyncInputResult{
			Name: input.Name, Digest: input.Digest, Size: input.Size, Cached: !downloaded[input.Name],
		})
	}
	sort.Slice(result.Inputs, func(i, j int) bool { return result.Inputs[i].Name < result.Inputs[j].Name })
	return result
}
