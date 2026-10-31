package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

type ArtifactSetSource struct{}

var isoArtifactPath = regexp.MustCompile(`^artifacts/agent\.[A-Za-z0-9_-]+\.iso$`)
var pxeArtifactPath = regexp.MustCompile(`^artifacts/pxe/agent\.([A-Za-z0-9_-]+)-(vmlinuz|initrd\.img|rootfs\.img)$`)
var pxeScriptPath = regexp.MustCompile(`^artifacts/pxe/agent\.([A-Za-z0-9_-]+)\.ipxe$`)

func (ArtifactSetSource) LoadAndVerify(manifestPath string) (ports.PublishedArtifactSet, error) {
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() || manifestInfo.Mode()&os.ModeSymlink != 0 {
		return ports.PublishedArtifactSet{}, errors.New("artifact manifest is missing or unsafe")
	}
	if filepath.Base(manifestPath) != "run.json" {
		return ports.PublishedArtifactSet{}, errors.New("artifact manifest must be a published run.json")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ports.PublishedArtifactSet{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	var manifest struct {
		SchemaVersion int               `json:"schemaVersion"`
		Outcome       string            `json:"outcome"`
		Artifacts     []domain.Artifact `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ports.PublishedArtifactSet{}, fmt.Errorf("decode artifact manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return ports.PublishedArtifactSet{}, errors.New("artifact manifest version is unsupported")
	}
	if manifest.Outcome != "success" || len(manifest.Artifacts) == 0 {
		return ports.PublishedArtifactSet{}, errors.New("artifact manifest does not describe a completed artifact set")
	}
	root, err := filepath.Abs(filepath.Dir(manifestPath))
	if err != nil {
		return ports.PublishedArtifactSet{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return ports.PublishedArtifactSet{}, fmt.Errorf("resolve artifact root: %w", err)
	}
	set := ports.PublishedArtifactSet{Root: root, Artifacts: make([]ports.PublishedArtifact, 0, len(manifest.Artifacts))}
	seen := make(map[string]struct{}, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if err := validatePublishedArtifact(artifact); err != nil {
			return ports.PublishedArtifactSet{}, err
		}
		if _, exists := seen[artifact.Path]; exists {
			return ports.PublishedArtifactSet{}, fmt.Errorf("artifact path %q is duplicated", artifact.Path)
		}
		seen[artifact.Path] = struct{}{}
		path := filepath.Join(root, filepath.FromSlash(artifact.Path))
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return ports.PublishedArtifactSet{}, fmt.Errorf("resolve artifact %q: %w", artifact.Path, err)
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || resolved != path {
			return ports.PublishedArtifactSet{}, fmt.Errorf("artifact %q escapes the published artifact set", artifact.Path)
		}
		if err := verifyPublishedArtifact(path, artifact); err != nil {
			return ports.PublishedArtifactSet{}, fmt.Errorf("verify artifact %q: %w", artifact.Path, err)
		}
		set.Artifacts = append(set.Artifacts, ports.PublishedArtifact{Artifact: artifact, FilePath: path})
	}
	if err := validateArtifactSetShape(seen); err != nil {
		return ports.PublishedArtifactSet{}, err
	}
	return set, nil
}

func validatePublishedArtifact(artifact domain.Artifact) error {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(artifact.Path)))
	if artifact.Path == "" || clean != artifact.Path || filepath.IsAbs(artifact.Path) || strings.Contains(artifact.Path, `\`) || !strings.HasPrefix(artifact.Path, "artifacts/") {
		return fmt.Errorf("artifact path %q is unsafe", artifact.Path)
	}
	if artifact.Size <= 0 {
		return fmt.Errorf("artifact %q has an invalid size", artifact.Path)
	}
	digest := strings.TrimPrefix(artifact.SHA256, "sha256:")
	decoded, err := hex.DecodeString(digest)
	if !strings.HasPrefix(artifact.SHA256, "sha256:") || err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("artifact %q has an invalid SHA-256 digest", artifact.Path)
	}
	return nil
}

func validateArtifactSetShape(paths map[string]struct{}) error {
	if len(paths) == 1 {
		for path := range paths {
			if isoArtifactPath.MatchString(path) {
				return nil
			}
		}
	}
	if len(paths) != 4 && len(paths) != 5 {
		return errors.New("artifact manifest contains an unsupported artifact set")
	}
	if _, found := paths["artifacts/pxe/manifest.json"]; !found {
		return errors.New("artifact manifest contains an incomplete PXE artifact set")
	}
	architecture := ""
	kinds := map[string]bool{}
	for path := range paths {
		if path == "artifacts/pxe/manifest.json" {
			continue
		}
		if match := pxeArtifactPath.FindStringSubmatch(path); len(match) == 3 {
			if architecture == "" {
				architecture = match[1]
			}
			if match[1] != architecture || kinds[match[2]] {
				return errors.New("artifact manifest contains an inconsistent PXE artifact set")
			}
			kinds[match[2]] = true
			continue
		}
		if match := pxeScriptPath.FindStringSubmatch(path); len(match) == 2 {
			if architecture == "" {
				architecture = match[1]
			}
			if match[1] != architecture || kinds["ipxe"] {
				return errors.New("artifact manifest contains an inconsistent PXE artifact set")
			}
			kinds["ipxe"] = true
			continue
		}
		return errors.New("artifact manifest contains an unsupported artifact path")
	}
	for _, kind := range []string{"vmlinuz", "initrd.img", "rootfs.img"} {
		if !kinds[kind] {
			return errors.New("artifact manifest contains an incomplete PXE artifact set")
		}
	}
	return nil
}

func verifyPublishedArtifact(path string, artifact domain.Artifact) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("file is missing or unsafe")
	}
	if info.Size() != artifact.Size {
		return errors.New("file size does not match the manifest")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return errors.New("file checksum does not match the manifest")
	}
	return nil
}
