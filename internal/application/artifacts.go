package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

type ArtifactBuilder struct {
	Validator   ConfigurationValidator
	Credentials ports.CredentialSource
	Cache       ports.ReleaseCache
	Extractor   ports.ToolExtractor
	Installer   ports.Installer
	Store       ports.ArtifactStore
	OkubVersion string
}

type ArtifactBuildResult struct {
	Action    string            `json:"action"`
	Type      string            `json:"type"`
	Artifacts []domain.Artifact `json:"artifacts"`
}

type RunMetadata struct {
	SchemaVersion        int                   `json:"schemaVersion"`
	Outcome              string                `json:"outcome"`
	ConfigSHA256         string                `json:"configSHA256"`
	OkubVersion          string                `json:"okubVersion"`
	InstallerVersion     string                `json:"installerVersion"`
	ReleaseVersion       string                `json:"releaseVersion"`
	ReleaseImageDigest   string                `json:"releaseImageDigest"`
	ReleaseImagePullSpec string                `json:"releaseImagePullSpec"`
	Inputs               []domain.ReleaseInput `json:"inputs"`
	Artifacts            []domain.Artifact     `json:"artifacts"`
	StartedAt            time.Time             `json:"startedAt"`
	FinishedAt           time.Time             `json:"finishedAt"`
	Invocation           []string              `json:"invocation"`
	LogPath              string                `json:"logPath"`
	ExitCode             int                   `json:"exitCode"`
}

type InstallerCommandError struct{ Err error }

func (e *InstallerCommandError) Error() string { return "openshift-install operation failed" }
func (e *InstallerCommandError) Unwrap() error { return e.Err }

func (b ArtifactBuilder) Build(ctx context.Context, configPath, outputOverride, artifactType string, force, offline bool) (ArtifactBuildResult, domain.ValidationReport, error) {
	if artifactType != "iso" && artifactType != "pxe" {
		return ArtifactBuildResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "artifact type must be iso or pxe"}
	}
	if offline {
		return ArtifactBuildResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "artifact build requires registry access; disconnected image layers are not cached"}
	}
	validator := b.Validator
	validator.OutputDirectoryOverride = outputOverride
	validated, err := validator.Validate(configPath)
	if err != nil || !validated.Report.Valid {
		return ArtifactBuildResult{}, validated.Report, err
	}
	c := validated.Definition
	if artifactType == "pxe" && c.Boot.Method != "pxe" {
		return ArtifactBuildResult{}, validated.Report, &CompatibilityError{Message: "PXE artifact build requires boot.method pxe and boot.artifactsBaseURL"}
	}
	release, found, err := b.Cache.Lookup(c.Artifacts.CacheDirectory, c.Cluster.Version, c.Cluster.Architecture)
	if err != nil || !found {
		if err == nil {
			err = errors.New("verified release is not cached")
		}
		return ArtifactBuildResult{}, validated.Report, &CacheError{Err: err}
	}
	if _, found := releaseInput(release, "client"); !found {
		return ArtifactBuildResult{}, validated.Report, &CacheError{Err: errors.New("cached client input is missing")}
	}
	if err := validateCachedRelease(release, c.Cluster.Version, c.Cluster.Architecture); err != nil {
		return ArtifactBuildResult{}, validated.Report, &CacheError{Err: err}
	}
	pullSecret, err := b.Credentials.ReadCredential(c.Credentials.PullSecretFile)
	if err != nil {
		return ArtifactBuildResult{}, validated.Report, &CredentialError{Path: c.Credentials.PullSecretFile, Err: err}
	}
	sshKey, err := b.Credentials.ReadCredential(c.Credentials.SSHPublicKeyFile)
	if err != nil {
		return ArtifactBuildResult{}, validated.Report, &CredentialError{Path: c.Credentials.SSHPublicKeyFile, Err: err}
	}
	inputs, err := renderInstallerFiles(c, strings.TrimSpace(string(pullSecret)), strings.TrimSpace(string(sshKey)))
	if err != nil {
		return ArtifactBuildResult{}, validated.Report, err
	}
	stage, err := b.Store.Prepare(c.Artifacts.OutputDirectory, inputs, force)
	if err != nil {
		return ArtifactBuildResult{}, validated.Report, &CacheError{Err: err}
	}
	defer b.Store.Discard(stage)
	installerPath := b.Store.InstallerPath(stage)
	ocPath := filepath.Join(stage.Root, "oc")
	for _, tool := range []struct {
		input, executable, destination string
	}{{"installer", "openshift-install", installerPath}, {"client", "oc", ocPath}} {
		input, found := releaseInput(release, tool.input)
		if !found {
			return ArtifactBuildResult{}, validated.Report, &CacheError{Err: fmt.Errorf("cached %s input is missing", tool.input)}
		}
		archive, err := b.Cache.OpenBlob(c.Artifacts.CacheDirectory, input)
		if err != nil {
			return ArtifactBuildResult{}, validated.Report, &CacheError{Err: fmt.Errorf("open cached %s input: %w", tool.input, err)}
		}
		extractErr := b.Extractor.ExtractExecutable(archive, tool.destination, tool.executable)
		closeErr := archive.Close()
		if extractErr != nil || closeErr != nil {
			return ArtifactBuildResult{}, validated.Report, &CacheError{Err: fmt.Errorf("extract cached %s executable: %w", tool.executable, errors.Join(extractErr, closeErr))}
		}
	}
	version, err := b.Installer.Version(ctx, installerPath)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ArtifactBuildResult{}, validated.Report, err
		}
		return ArtifactBuildResult{}, validated.Report, &CompatibilityError{Message: "cached openshift-install version is incompatible with the resolved release"}
	}
	if installerVersion(version) != release.InstallerVersion {
		return ArtifactBuildResult{}, validated.Report, &CompatibilityError{Message: "cached openshift-install version is incompatible with the resolved release"}
	}
	create := "image"
	if artifactType == "pxe" {
		create = "pxe-files"
	}
	args := []string{"agent", "create", create, "--dir", stage.Work}
	commandPath := stage.Root
	if inheritedPath := os.Getenv("PATH"); inheritedPath != "" {
		commandPath += string(os.PathListSeparator) + inheritedPath
	}
	command := ports.InstallerCommand{
		Args: args, Directory: stage.Work,
		Environment: map[string]string{"OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE": release.ImagePullSpec, "PATH": commandPath},
		Secrets:     pullSecretRedactions(pullSecret, stage.Root),
	}
	result, commandErr := b.Installer.Run(ctx, installerPath, command)
	if commandErr != nil {
		_ = b.Store.WriteFailureLog(c.Artifacts.OutputDirectory, result.Log)
		return ArtifactBuildResult{}, validated.Report, &InstallerCommandError{Err: commandErr}
	}
	for _, toolPath := range []string{installerPath, ocPath} {
		if err := os.Remove(toolPath); err != nil {
			return ArtifactBuildResult{}, validated.Report, &CacheError{Err: fmt.Errorf("remove staged tool: %w", err)}
		}
	}
	artifacts, err := b.Store.FinalizeArtifacts(stage, artifactType, c.Cluster.Architecture)
	if err != nil {
		_ = b.Store.WriteFailureLog(c.Artifacts.OutputDirectory, result.Log)
		return ArtifactBuildResult{}, validated.Report, &InstallerCommandError{Err: err}
	}
	configJSON, _ := json.Marshal(c)
	configDigest := sha256.Sum256(configJSON)
	metadata := RunMetadata{
		SchemaVersion: 1, Outcome: "success", ConfigSHA256: "sha256:" + hex.EncodeToString(configDigest[:]), OkubVersion: b.OkubVersion,
		InstallerVersion: release.InstallerVersion, ReleaseVersion: release.Version, ReleaseImageDigest: release.ImageDigest,
		ReleaseImagePullSpec: release.ImagePullSpec, Inputs: append([]domain.ReleaseInput(nil), release.Inputs...), Artifacts: artifacts,
		StartedAt: result.StartedAt, FinishedAt: result.FinishedAt,
		Invocation: []string{"openshift-install", "agent", "create", create, "--dir", "<private-staging>"}, LogPath: "logs/openshift-install.log",
		ExitCode: result.ExitCode,
	}
	sort.Slice(metadata.Inputs, func(i, j int) bool { return metadata.Inputs[i].Name < metadata.Inputs[j].Name })
	runJSON, err := json.Marshal(metadata)
	if err != nil {
		return ArtifactBuildResult{}, validated.Report, err
	}
	runJSON = append(runJSON, '\n')
	if err := b.Store.WriteResultFiles(stage, result.Log, runJSON); err != nil {
		return ArtifactBuildResult{}, validated.Report, &CacheError{Err: err}
	}
	action, err := b.Store.Publish(c.Artifacts.OutputDirectory, stage, force)
	if err != nil {
		return ArtifactBuildResult{}, validated.Report, &CacheError{Err: err}
	}
	return ArtifactBuildResult{Action: action, Type: artifactType, Artifacts: artifacts}, validated.Report, nil
}

func pullSecretRedactions(content []byte, additional ...string) []string {
	values := append([]string{string(content), strings.TrimSpace(string(content))}, additional...)
	var decoded any
	if json.Unmarshal(content, &decoded) == nil {
		var collect func(any)
		collect = func(value any) {
			switch value := value.(type) {
			case string:
				values = append(values, value)
			case []any:
				for _, item := range value {
					collect(item)
				}
			case map[string]any:
				for _, item := range value {
					collect(item)
				}
			}
		}
		collect(decoded)
	}
	unique := make(map[string]struct{}, len(values))
	rawValues := values
	values = values[:0]
	for _, value := range rawValues {
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	for value := range unique {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}
		return values[i] < values[j]
	})
	return values
}

var exactInstallerVersion = regexp.MustCompile(`(?m)^(?:.*[/])?openshift-install[ \t]+(\S+)[ \t]*$`)

func installerVersion(output string) string {
	match := exactInstallerVersion.FindStringSubmatch(output)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func releaseInput(release domain.Release, name string) (domain.ReleaseInput, bool) {
	for _, input := range release.Inputs {
		if input.Name == name {
			return input, true
		}
	}
	return domain.ReleaseInput{}, false
}
