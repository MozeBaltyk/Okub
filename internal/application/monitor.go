package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

type InstallMonitor struct {
	Validator   ConfigurationValidator
	Credentials ports.CredentialSource
	Cache       ports.ReleaseCache
	Extractor   ports.InstallerExtractor
	Installer   ports.Installer
	Store       ports.ArtifactStore
}

type MonitorResult struct {
	Action     string    `json:"action"`
	Outcome    string    `json:"outcome"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	LogPath    string    `json:"logPath"`
	Invocation []string  `json:"invocation"`
}

func (m InstallMonitor) Monitor(ctx context.Context, configPath, outputOverride string, force bool) (MonitorResult, domain.ValidationReport, error) {
	validator := m.Validator
	validator.OutputDirectoryOverride = outputOverride
	validated, err := validator.Validate(configPath)
	if err != nil || !validated.Report.Valid {
		return MonitorResult{}, validated.Report, err
	}
	c := validated.Definition
	release, found, err := m.Cache.Lookup(c.Artifacts.CacheDirectory, c.Cluster.Version, c.Cluster.Architecture)
	if err != nil || !found {
		if err == nil {
			err = errors.New("verified release is not cached")
		}
		return MonitorResult{}, validated.Report, &CacheError{Err: err}
	}
	if err := validateCachedRelease(release, c.Cluster.Version, c.Cluster.Architecture); err != nil {
		return MonitorResult{}, validated.Report, &CacheError{Err: err}
	}
	pullSecret, err := m.Credentials.ReadCredential(c.Credentials.PullSecretFile)
	if err != nil {
		return MonitorResult{}, validated.Report, &CredentialError{Path: c.Credentials.PullSecretFile, Err: err}
	}
	stage, err := m.Store.PrepareMonitor(c.Artifacts.OutputDirectory)
	if err != nil {
		return MonitorResult{}, validated.Report, &CacheError{Err: err}
	}
	defer m.Store.Discard(stage)
	action, err := m.Store.MonitorAction(stage)
	if err != nil {
		return MonitorResult{}, validated.Report, &CacheError{Err: err}
	}
	if action == "update" && !force {
		return MonitorResult{}, validated.Report, &CacheError{Err: errors.New("monitor result exists; use --force to replace it")}
	}
	input, found := releaseInput(release, "installer")
	if !found {
		return MonitorResult{}, validated.Report, &CacheError{Err: errors.New("cached installer input is missing")}
	}
	archive, err := m.Cache.OpenBlob(c.Artifacts.CacheDirectory, input)
	if err != nil {
		return MonitorResult{}, validated.Report, &CacheError{Err: err}
	}
	extractErr := m.Extractor.Extract(archive, m.Store.InstallerPath(stage))
	closeErr := archive.Close()
	if extractErr != nil || closeErr != nil {
		return MonitorResult{}, validated.Report, &CacheError{Err: errors.Join(extractErr, closeErr)}
	}
	version, err := m.Installer.Version(ctx, m.Store.InstallerPath(stage))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return MonitorResult{}, validated.Report, err
		}
		return MonitorResult{}, validated.Report, &CompatibilityError{Message: "cached openshift-install version is incompatible with the resolved release"}
	}
	if installerVersion(version) != release.InstallerVersion {
		return MonitorResult{}, validated.Report, &CompatibilityError{Message: "cached openshift-install version is incompatible with the resolved release"}
	}
	command := ports.InstallerCommand{
		Args: []string{"agent", "wait-for", "install-complete", "--dir", stage.Work}, Directory: stage.Work,
		Environment: map[string]string{"OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE": release.ImagePullSpec},
		Secrets:     pullSecretRedactions(pullSecret, stage.Root, stage.Work),
	}
	installed, commandErr := m.Installer.Run(ctx, m.Store.InstallerPath(stage), command)
	outcome := "success"
	if commandErr != nil {
		outcome = "failed"
		if errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) {
			outcome = "cancelled"
		}
	}
	result := MonitorResult{
		Action:  action,
		Outcome: outcome, StartedAt: installed.StartedAt, FinishedAt: installed.FinishedAt,
		LogPath:    "monitor/openshift-install.log",
		Invocation: []string{"openshift-install", "agent", "wait-for", "install-complete", "--dir", "<published-installer-state>"},
	}
	data, _ := json.Marshal(result)
	data = append(data, '\n')
	publishedAction, publishErr := m.Store.PublishMonitor(c.Artifacts.OutputDirectory, stage, installed.Log, data, force)
	if publishErr != nil {
		return MonitorResult{}, validated.Report, &CacheError{Err: publishErr}
	}
	result.Action = publishedAction
	if commandErr != nil {
		return result, validated.Report, &InstallerCommandError{Err: commandErr}
	}
	return result, validated.Report, nil
}
