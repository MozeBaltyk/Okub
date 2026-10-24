// Package cli provides the command-line presentation for Okub.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	cacheadapter "github.com/mozebaltyk/Okub/internal/adapters/cache"
	"github.com/mozebaltyk/Okub/internal/adapters/coreos"
	dnsadapter "github.com/mozebaltyk/Okub/internal/adapters/dns"
	"github.com/mozebaltyk/Okub/internal/adapters/environment"
	"github.com/mozebaltyk/Okub/internal/adapters/filesystem"
	helperadapter "github.com/mozebaltyk/Okub/internal/adapters/helper"
	"github.com/mozebaltyk/Okub/internal/adapters/openshift"
	"github.com/mozebaltyk/Okub/internal/application"
	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/spf13/cobra"
)

const (
	exitValidation   = 1
	exitInvalidUsage = 2
	exitExternal     = 3
	exitNetwork      = 4
	exitFilesystem   = 5
	exitInterrupted  = 130
)

const (
	releaseControllerURL = "https://amd64.origin.releases.ci.openshift.org/api/v1/releasestream/4-scos-stable/tags"
	githubAPIURL         = "https://api.github.com"
)

type commandError struct {
	code  int
	err   error
	quiet bool
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

// Execute runs the Okub CLI and returns a process exit code.
func Execute(args []string, version string, stdout, stderr io.Writer) int {
	workspace, err := workspaceRoot()
	if err != nil {
		fmt.Fprintf(stderr, "okub: find workspace root: %v\n", err)
		return exitFilesystem
	}
	cache := cacheadapter.Release{}
	preflightFactory := func(validator application.ConfigurationValidator) application.Preflight {
		return application.Preflight{Validator: validator, DNS: dnsadapter.Checker{Timeout: 3 * time.Second}, Environment: environment.Local{Timeout: 4 * time.Second}, Cache: cache}
	}
	cmd := newRootCommand(version, commandFactories{
		preflight: func(validator application.ConfigurationValidator) application.Preflight {
			return preflightFactory(validator)
		},
		images: func(validator application.ConfigurationValidator) application.ImageSync {
			return application.ImageSync{
				Validator: validator, Resolver: coreos.Resolver{ReleaseControllerURL: releaseControllerURL, GitHubAPIURL: githubAPIURL},
				Downloader: coreos.HTTPDownloader{}, Cache: cache, Extractor: openshift.Extractor{}, ToolsDirectory: filepath.Join(workspace, ".local", "tools"),
			}
		},
		artifacts: func(validator application.ConfigurationValidator) application.ArtifactBuilder {
			return application.ArtifactBuilder{
				Validator: validator, Credentials: filesystem.ConfigurationSource{}, Cache: cache,
				Extractor: openshift.Extractor{}, Installer: openshift.Runner{}, Store: filesystem.ArtifactStore{}, OkubVersion: version,
			}
		},
		monitor: func(validator application.ConfigurationValidator) application.InstallMonitor {
			return application.InstallMonitor{
				Validator: validator, Credentials: filesystem.ConfigurationSource{}, Cache: cache,
				Extractor: openshift.Extractor{}, Installer: openshift.Runner{}, Store: filesystem.ArtifactStore{},
			}
		},
	})
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return executeCommand(cmd, ctx, stderr)
}

func executeCommand(cmd *cobra.Command, ctx context.Context, stderr io.Writer) int {
	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return exitInterrupted
		}
		var commandErr *commandError
		if errors.As(err, &commandErr) {
			if !commandErr.quiet {
				fmt.Fprintf(stderr, "okub: %v\n", commandErr.err)
			}
			return commandErr.code
		}
		fmt.Fprintf(stderr, "okub: %v\n", err)
		return exitInvalidUsage
	}
	return 0
}

type preflightFactory func(application.ConfigurationValidator) application.Preflight
type imageSyncFactory func(application.ConfigurationValidator) application.ImageSync
type artifactBuilderFactory func(application.ConfigurationValidator) application.ArtifactBuilder
type monitorFactory func(application.ConfigurationValidator) application.InstallMonitor

type commandFactories struct {
	preflight preflightFactory
	images    imageSyncFactory
	artifacts artifactBuilderFactory
	monitor   monitorFactory
	helper    func() helperadapter.Manager
}

func newRootCommand(version string, factories commandFactories) *cobra.Command {
	var configPath string
	var outputPath string
	var jsonOutput, dryRun, offline bool
	root := &cobra.Command{
		Use:           "okub",
		Short:         "Prepare, validate, and bootstrap bare-metal OKD clusters",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate("okub {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVar(&configPath, "config", "./okub.yaml", "path to the cluster definition")
	root.PersistentFlags().StringVar(&outputPath, "output", "", "override the configured output directory")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit one JSON result")
	root.PersistentFlags().BoolVar(&offline, "offline", false, "forbid network access and require verified cached inputs")
	root.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "report intended writes without changing files")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if dryRun && cmd.Name() != "render" {
			return &commandError{code: exitInvalidUsage, err: errors.New("--dry-run is only supported by commands that write state")}
		}
		return nil
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the Okub version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "okub %s\n", version)
		},
	})

	config := &cobra.Command{Use: "config", Short: "Inspect cluster configuration"}
	config.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate configuration without environmental checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workflow, err := configurationValidator()
			if err != nil {
				return err
			}
			result, err := workflow.Validate(configPath)
			if err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			if err := writeReport(cmd.OutOrStdout(), result.Report, jsonOutput); err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", err)}
			}
			if !result.Report.Valid {
				return &commandError{code: validationExitCode(result.Report), err: errors.New("configuration is invalid"), quiet: true}
			}
			return nil
		},
	})
	var force bool
	render := &cobra.Command{
		Use:   "render",
		Short: "Render Agent-based Installer configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			validator, err := configurationValidator()
			if err != nil {
				return err
			}
			workflow := application.InstallerInputRenderer{
				Validator: validator, Credentials: filesystem.ConfigurationSource{}, Publisher: filesystem.Publisher{},
			}
			result, report, err := workflow.Render(configPath, outputPath, force, dryRun)
			if err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			if !report.Valid {
				if err := writeReport(cmd.OutOrStdout(), report, jsonOutput); err != nil {
					return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", err)}
				}
				return &commandError{code: validationExitCode(report), err: errors.New("configuration is invalid"), quiet: true}
			}
			if err := writeRenderResult(cmd.OutOrStdout(), result, jsonOutput); err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", err)}
			}
			return nil
		},
	}
	render.Flags().BoolVar(&force, "force", false, "replace differing generated output")
	config.AddCommand(render)
	root.AddCommand(config)

	preflight := &cobra.Command{Use: "preflight", Short: "Inspect installation prerequisites"}
	preflight.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run read-only configuration and environment checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			validator, err := configurationValidator()
			if err != nil {
				return err
			}
			report, err := factories.preflight(validator).Run(cmd.Context(), configPath, outputPath, offline)
			if err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			if err := writePreflightReport(cmd.OutOrStdout(), report, jsonOutput); err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", err)}
			}
			if !report.Valid {
				return &commandError{code: exitValidation, err: errors.New("preflight failed"), quiet: true}
			}
			return nil
		},
	})
	root.AddCommand(preflight)

	images := &cobra.Command{Use: "images", Short: "Manage verified release inputs"}
	images.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Resolve and cache verified OKD release inputs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			validator, err := configurationValidator()
			if err != nil {
				return err
			}
			result, report, err := factories.images(validator).Sync(cmd.Context(), configPath, offline)
			if !report.Valid && len(report.Findings) != 0 {
				if writeErr := writeReport(cmd.OutOrStdout(), report, jsonOutput); writeErr != nil {
					return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", writeErr)}
				}
				return &commandError{code: validationExitCode(report), err: errors.New("configuration is invalid"), quiet: true}
			}
			if err != nil {
				return imageSyncCommandError(err)
			}
			if err := writeImageSyncResult(cmd.OutOrStdout(), result, jsonOutput); err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", err)}
			}
			return nil
		},
	})
	root.AddCommand(images)

	var artifactType string
	var artifactForce bool
	artifacts := &cobra.Command{Use: "artifacts", Short: "Build verified boot artifacts"}
	build := &cobra.Command{
		Use:   "build",
		Short: "Build Agent ISO or PXE artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if artifactType != "iso" && artifactType != "pxe" {
				return &commandError{code: exitInvalidUsage, err: errors.New("artifact type must be iso or pxe")}
			}
			if offline {
				return artifactCommandError(&application.CompatibilityError{Message: "artifact build requires registry access; disconnected image layers are not cached"})
			}
			validator, err := configurationValidator()
			if err != nil {
				return err
			}
			preflight, err := factories.preflight(validator).Run(cmd.Context(), configPath, outputPath, false)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return artifactCommandError(err)
				}
				return &commandError{code: exitFilesystem, err: err}
			}
			if cmd.Context().Err() != nil {
				return artifactCommandError(cmd.Context().Err())
			}
			if !preflight.Valid {
				if writeErr := writePreflightReport(cmd.OutOrStdout(), preflight, jsonOutput); writeErr != nil {
					return &commandError{code: exitFilesystem, err: writeErr}
				}
				return &commandError{code: exitValidation, err: errors.New("preflight failed"), quiet: true}
			}
			result, report, err := factories.artifacts(validator).Build(cmd.Context(), configPath, outputPath, artifactType, artifactForce, false)
			if !report.Valid && len(report.Findings) != 0 {
				if writeErr := writeReport(cmd.OutOrStdout(), report, jsonOutput); writeErr != nil {
					return &commandError{code: exitFilesystem, err: writeErr}
				}
				return &commandError{code: exitValidation, err: errors.New("artifact prerequisites failed"), quiet: true}
			}
			if err != nil {
				return artifactCommandError(err)
			}
			if err := writeArtifactResult(cmd.OutOrStdout(), result, jsonOutput); err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			return nil
		},
	}
	build.Flags().StringVar(&artifactType, "type", "", "artifact type: iso or pxe")
	build.Flags().BoolVar(&artifactForce, "force", false, "replace an existing completed output")
	_ = build.MarkFlagRequired("type")
	artifacts.AddCommand(build)
	root.AddCommand(artifacts)

	var monitorForce bool
	install := &cobra.Command{Use: "install", Short: "Observe an Agent installation"}
	monitor := &cobra.Command{
		Use:   "monitor",
		Short: "Wait for installation completion",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if offline {
				return &commandError{code: exitInvalidUsage, err: errors.New("install monitor cannot run offline")}
			}
			validator, err := configurationValidator()
			if err != nil {
				return err
			}
			result, report, err := factories.monitor(validator).Monitor(cmd.Context(), configPath, outputPath, monitorForce)
			if !report.Valid && len(report.Findings) != 0 {
				if writeErr := writeReport(cmd.OutOrStdout(), report, jsonOutput); writeErr != nil {
					return &commandError{code: exitFilesystem, err: writeErr}
				}
				return &commandError{code: exitValidation, err: errors.New("configuration is invalid"), quiet: true}
			}
			if err != nil {
				return artifactCommandError(err)
			}
			if jsonOutput {
				err = json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "installation monitor %s\n", result.Outcome)
			}
			return err
		},
	}
	monitor.Flags().BoolVar(&monitorForce, "force", false, "replace an existing monitor result")
	install.AddCommand(monitor)
	root.AddCommand(install)

	var helperApply bool
	helper := &cobra.Command{Use: "helper", Short: "Manage the optional Fedora network-services helper"}
	helperImage := &cobra.Command{Use: "image", Short: "Manage the verified Fedora Cloud image"}
	helperImage.AddCommand(&cobra.Command{Use: "sync", Short: "Download and verify Fedora Cloud 44", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		validator, err := configurationValidator()
		if err != nil {
			return err
		}
		validated, err := validator.Validate(configPath)
		if err != nil {
			return &commandError{code: exitFilesystem, err: err}
		}
		if !validated.Report.Valid {
			_ = writeReport(cmd.OutOrStdout(), validated.Report, jsonOutput)
			return &commandError{code: exitValidation, err: errors.New("configuration is invalid"), quiet: true}
		}
		if validated.Definition.Helper == nil {
			return &commandError{code: exitValidation, err: errors.New("helper configuration is required")}
		}
		repo, err := repositoryRoot()
		if err != nil {
			return &commandError{code: exitFilesystem, err: err}
		}
		result, err := (helperadapter.ImageSync{}).Sync(cmd.Context(), repo, offline)
		if err != nil {
			return &commandError{code: exitNetwork, err: err}
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "verified Fedora Cloud image %s (sha256:%s, %d bytes)\n", result.ImagePath, result.SHA256, result.Size)
		return err
	}})
	helper.AddCommand(helperImage)
	helper.AddCommand(&cobra.Command{Use: "render", Short: "Render deterministic Fedora helper cloud-init", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if dryRun {
			return &commandError{code: exitInvalidUsage, err: errors.New("--dry-run is not supported by helper render")}
		}
		validator, err := configurationValidator()
		if err != nil {
			return err
		}
		repo, err := repositoryRoot()
		if err != nil {
			return &commandError{code: exitFilesystem, err: err}
		}
		result, report, err := application.RenderHelper(configPath, repo, validator)
		if !report.Valid {
			_ = writeReport(cmd.OutOrStdout(), report, jsonOutput)
			return &commandError{code: exitValidation, err: errors.New("configuration is invalid"), quiet: true}
		}
		if err != nil {
			return &commandError{code: exitFilesystem, err: err}
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "rendered Fedora helper cloud-init at %s\n", result.Directory)
		return err
	}})
	for _, operation := range []string{"up", "down"} {
		operation := operation
		lifecycle := &cobra.Command{Use: operation, Short: "Plan or explicitly apply the Fedora helper " + operation + " lifecycle", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			validator, err := configurationValidator()
			if err != nil {
				return err
			}
			validated, err := validator.Validate(configPath)
			if err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			if !validated.Report.Valid {
				_ = writeReport(cmd.OutOrStdout(), validated.Report, jsonOutput)
				return &commandError{code: exitValidation, err: errors.New("configuration is invalid"), quiet: true}
			}
			if validated.Definition.Helper == nil {
				return &commandError{code: exitValidation, err: errors.New("helper configuration is required")}
			}
			repo, err := repositoryRoot()
			if err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			manager := helperadapter.Manager{}
			if factories.helper != nil {
				manager = factories.helper()
			}
			result, err := manager.Run(cmd.Context(), repo, validated.Definition, operation, helperApply)
			if err != nil {
				return &commandError{code: exitExternal, err: err}
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return err
		}}
		lifecycle.Flags().BoolVar(&helperApply, "apply", false, "authorize the inspected safe operation (plan/read-only by default)")
		helper.AddCommand(lifecycle)
	}
	root.AddCommand(helper)

	return root
}

func repositoryRoot() (string, error) {
	current, err := workspaceRoot()
	if err != nil {
		return "", err
	}
	if info, e := os.Stat(filepath.Join(current, "lab", "fedora-helper", "main.tf")); e == nil && info.Mode().IsRegular() {
		return current, nil
	}
	return "", errors.New("repository root containing lab/fedora-helper was not found")
}

func workspaceRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return workspaceRootFrom(current), nil
}

func workspaceRootFrom(start string) string {
	current, err := filepath.Abs(start)
	if err != nil {
		return filepath.Clean(start)
	}
	fallback := current
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		if info, err := os.Stat(filepath.Join(current, "go.mod")); err == nil && info.Mode().IsRegular() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fallback
		}
		current = parent
	}
}

func configurationValidator() (application.ConfigurationValidator, error) {
	workspace, err := workspaceRoot()
	if err != nil {
		return application.ConfigurationValidator{}, &commandError{code: exitFilesystem, err: fmt.Errorf("find workspace root: %w", err)}
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return application.ConfigurationValidator{}, &commandError{code: exitFilesystem, err: fmt.Errorf("find user home directory: %w", err)}
	}
	return application.ConfigurationValidator{
		Source: filesystem.ConfigurationSource{}, OutputDirectoryRoot: filepath.Join(workspace, ".local", "build"), CacheDirectory: filepath.Join(workspace, ".local", "cache"), HomeDirectory: homeDirectory,
	}, nil
}

func validationExitCode(report domain.ValidationReport) int {
	for _, finding := range report.Findings {
		if finding.ID == "apiVersion.unsupported" {
			return exitInvalidUsage
		}
	}
	return exitValidation
}

func writeRenderResult(w io.Writer, result application.RenderResult, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(w).Encode(result)
	}
	verb := map[string]string{"create": "created", "update": "updated", "unchanged": "unchanged"}[result.Action]
	if result.DryRun && result.Action != "unchanged" {
		verb = "would " + result.Action
	}
	_, err := fmt.Fprintf(w, "%s installer configuration in %s\n", verb, result.OutputDirectory)
	return err
}

func writeReport(w io.Writer, report domain.ValidationReport, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(w).Encode(report)
	}
	if report.Valid {
		_, err := fmt.Fprintln(w, "configuration is valid")
		return err
	}
	for _, finding := range report.Findings {
		if _, err := fmt.Fprintf(w, "%s [%s] %s: %s\n", finding.Severity, finding.ID, finding.Path, finding.Message); err != nil {
			return err
		}
	}
	return nil
}

func writePreflightReport(w io.Writer, report domain.PreflightReport, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(w).Encode(report)
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(w, "%s [%s] %s: %s\n", check.Status, check.ID, check.Path, check.Message); err != nil {
			return err
		}
	}
	if report.Valid {
		_, err := fmt.Fprintln(w, "preflight passed")
		return err
	}
	return nil
}

func imageSyncCommandError(err error) error {
	var compatibility *application.CompatibilityError
	var network *application.NetworkError
	if errors.As(err, &compatibility) {
		return &commandError{code: exitExternal, err: err}
	}
	if errors.As(err, &network) {
		return &commandError{code: exitNetwork, err: err}
	}
	return &commandError{code: exitFilesystem, err: err}
}

func writeImageSyncResult(w io.Writer, result application.ImageSyncResult, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(w).Encode(result)
	}
	action := "cached"
	if result.Cached {
		action = "already verified"
	}
	_, err := fmt.Fprintf(w, "%s release %s for %s (%d inputs)\n", action, result.ReleaseVersion, result.Architecture, len(result.Inputs))
	return err
}

func artifactCommandError(err error) error {
	var installer *application.InstallerCommandError
	var compatibility *application.CompatibilityError
	if errors.As(err, &installer) || errors.As(err, &compatibility) {
		return &commandError{code: exitExternal, err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &commandError{code: exitExternal, err: err}
	}
	return &commandError{code: exitFilesystem, err: err}
}

func writeArtifactResult(w io.Writer, result application.ArtifactBuildResult, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(w).Encode(result)
	}
	_, err := fmt.Fprintf(w, "%s %s artifact set (%d files)\n", result.Action, result.Type, len(result.Artifacts))
	return err
}
