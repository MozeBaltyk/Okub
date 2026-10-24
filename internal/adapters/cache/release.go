// Package cache provides the verified content-addressed release cache.
package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
	"golang.org/x/sys/unix"
)

const manifestSchemaVersion = 2

var safePathPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type manifest struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	RequestedVersion string                `json:"requestedVersion"`
	Version          string                `json:"version"`
	Architecture     string                `json:"architecture"`
	InstallerVersion string                `json:"installerVersion"`
	ImagePullSpec    string                `json:"imagePullSpec"`
	ImageDigest      string                `json:"imageDigest"`
	Inputs           []domain.ReleaseInput `json:"inputs"`
}

// Release is a filesystem-backed, SHA-256-addressed cache.
type Release struct{}

func (Release) Lookup(root, requestedVersion, architecture string) (domain.Release, bool, error) {
	path, err := manifestPath(root, requestedVersion, architecture)
	if err != nil {
		return domain.Release{}, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Release{}, false, nil
	}
	if err != nil {
		return domain.Release{}, false, fmt.Errorf("inspect release cache manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return domain.Release{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Release{}, false, fmt.Errorf("read release cache manifest: %w", err)
	}
	var stored manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&stored) != nil || decoder.Decode(&struct{}{}) != io.EOF || stored.SchemaVersion != manifestSchemaVersion || stored.RequestedVersion != requestedVersion || stored.Architecture != architecture {
		return domain.Release{}, false, nil
	}
	release := domain.Release{
		RequestedVersion: stored.RequestedVersion, Version: stored.Version, Architecture: stored.Architecture,
		InstallerVersion: stored.InstallerVersion, ImagePullSpec: stored.ImagePullSpec,
		ImageDigest: stored.ImageDigest, Inputs: stored.Inputs,
	}
	if err := validateManifestRelease(release); err != nil {
		return domain.Release{}, false, nil
	}
	for _, input := range release.Inputs {
		verified, err := verifyBlob(root, input)
		if err != nil {
			return domain.Release{}, false, err
		}
		if !verified {
			return domain.Release{}, false, nil
		}
	}
	return release, true, nil
}

func (Release) HasBlob(root string, input domain.ReleaseInput) (bool, error) {
	if err := validateInput(input); err != nil {
		return false, err
	}
	return verifyBlob(root, input)
}

func (Release) OpenBlob(root string, input domain.ReleaseInput) (io.ReadCloser, error) {
	verified, err := verifyBlob(root, input)
	if err != nil {
		return nil, err
	}
	if !verified {
		return nil, errors.New("cache blob is not verified")
	}
	path, _ := blobPath(root, input.Digest)
	return os.Open(path)
}

func (Release) StoreBlob(root string, input domain.ReleaseInput, reader io.Reader) error {
	if err := validateInput(input); err != nil {
		return err
	}
	directory := filepath.Join(root, "blobs", "sha256")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create cache blob directory: %w", err)
	}
	stage, err := os.CreateTemp(directory, ".download-")
	if err != nil {
		return fmt.Errorf("create cache staging file: %w", err)
	}
	stagePath := stage.Name()
	defer os.Remove(stagePath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(stage, hash), io.LimitReader(reader, input.Size+1))
	syncErr := stage.Sync()
	closeErr := stage.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("stage cache blob: %w", errors.Join(copyErr, syncErr, closeErr))
	}
	if written != input.Size || hex.EncodeToString(hash.Sum(nil)) != digestHex(input.Digest) {
		return fmt.Errorf("%w: size or checksum mismatch", ports.ErrIntegrity)
	}
	if err := os.Chmod(stagePath, 0o644); err != nil {
		return fmt.Errorf("set cache blob permissions: %w", err)
	}
	target, _ := blobPath(root, input.Digest)
	if err := unix.Renameat2(unix.AT_FDCWD, stagePath, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE); err == nil {
		return nil
	} else if !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("publish cache blob: %w", err)
	}
	verified, err := verifyBlob(root, input)
	if err != nil {
		return err
	}
	if verified {
		return nil
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stagePath, unix.AT_FDCWD, target, unix.RENAME_EXCHANGE); err != nil {
		return fmt.Errorf("replace corrupt cache blob: %w", err)
	}
	return nil
}

func (Release) Publish(root string, release domain.Release) error {
	if err := validateManifestRelease(release); err != nil {
		return err
	}
	for _, input := range release.Inputs {
		verified, err := verifyBlob(root, input)
		if err != nil {
			return err
		}
		if !verified {
			return errors.New("cannot publish release manifest with an unverified blob")
		}
	}
	release.Inputs = append([]domain.ReleaseInput(nil), release.Inputs...)
	sort.Slice(release.Inputs, func(i, j int) bool { return release.Inputs[i].Name < release.Inputs[j].Name })
	stored := manifest{
		SchemaVersion: manifestSchemaVersion, RequestedVersion: release.RequestedVersion, Version: release.Version,
		Architecture: release.Architecture, InstallerVersion: release.InstallerVersion,
		ImagePullSpec: release.ImagePullSpec, ImageDigest: release.ImageDigest, Inputs: release.Inputs,
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode release cache manifest: %w", err)
	}
	data = append(data, '\n')
	target, err := manifestPath(root, release.RequestedVersion, release.Architecture)
	if err != nil {
		return err
	}
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create release cache directory: %w", err)
	}
	stage, err := os.CreateTemp(directory, ".manifest-")
	if err != nil {
		return fmt.Errorf("create manifest staging file: %w", err)
	}
	stagePath := stage.Name()
	defer os.Remove(stagePath)
	if _, err := stage.Write(data); err != nil {
		stage.Close()
		return fmt.Errorf("stage release manifest: %w", err)
	}
	if err := stage.Sync(); err != nil {
		stage.Close()
		return fmt.Errorf("sync release manifest: %w", err)
	}
	if err := stage.Close(); err != nil {
		return fmt.Errorf("close release manifest: %w", err)
	}
	if err := os.Chmod(stagePath, 0o644); err != nil {
		return fmt.Errorf("set release manifest permissions: %w", err)
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stagePath, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE); err == nil {
		return nil
	} else if !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("publish release manifest: %w", err)
	}
	_, found, lookupErr := (Release{}).Lookup(root, release.RequestedVersion, release.Architecture)
	if lookupErr != nil {
		return lookupErr
	}
	if found {
		return nil
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stagePath, unix.AT_FDCWD, target, unix.RENAME_EXCHANGE); err != nil {
		return fmt.Errorf("replace release manifest: %w", err)
	}
	return nil
}

func verifyBlob(root string, input domain.ReleaseInput) (bool, error) {
	path, err := blobPath(root, input.Digest)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect cache blob: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != input.Size {
		return false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open cache blob: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return false, fmt.Errorf("verify cache blob: %w", errors.Join(copyErr, closeErr))
	}
	return hex.EncodeToString(hash.Sum(nil)) == digestHex(input.Digest), nil
}

func manifestPath(root, version, architecture string) (string, error) {
	if root == "" || !safePathPart.MatchString(version) || !safePathPart.MatchString(architecture) {
		return "", errors.New("cache release identity is invalid")
	}
	return filepath.Join(root, "releases", version, architecture, "manifest.json"), nil
}

func blobPath(root, digest string) (string, error) {
	hexDigest := digestHex(digest)
	if root == "" || len(hexDigest) != 64 {
		return "", errors.New("cache digest is invalid")
	}
	if _, err := hex.DecodeString(hexDigest); err != nil {
		return "", errors.New("cache digest is invalid")
	}
	return filepath.Join(root, "blobs", "sha256", hexDigest), nil
}

func digestHex(digest string) string { return strings.TrimPrefix(strings.ToLower(digest), "sha256:") }

func validateManifestRelease(release domain.Release) error {
	if !safePathPart.MatchString(release.RequestedVersion) || !safePathPart.MatchString(release.Version) || !safePathPart.MatchString(release.Architecture) || !safePathPart.MatchString(release.InstallerVersion) {
		return errors.New("release identity is invalid")
	}
	if len(release.Inputs) != 3 {
		return errors.New("release must contain installer, client, and release-metadata inputs")
	}
	imageDigest := digestHex(release.ImageDigest)
	if len(imageDigest) != 64 || !strings.HasSuffix(strings.ToLower(release.ImagePullSpec), "@sha256:"+imageDigest) {
		return errors.New("release image metadata is invalid")
	}
	if _, err := hex.DecodeString(imageDigest); err != nil {
		return errors.New("release image metadata is invalid")
	}
	names := map[string]bool{}
	for _, input := range release.Inputs {
		if err := validateInput(input); err != nil {
			return err
		}
		names[input.Name] = true
	}
	if !names["installer"] || !names["client"] || !names["release-metadata"] {
		return errors.New("release must contain installer, client, and release-metadata inputs")
	}
	return nil
}

func validateInput(input domain.ReleaseInput) error {
	if input.Name != "installer" && input.Name != "client" && input.Name != "release-metadata" || input.Size <= 0 || input.Size == math.MaxInt64 {
		return errors.New("release input metadata is invalid")
	}
	if _, err := blobPath(".", input.Digest); err != nil {
		return err
	}
	return nil
}
