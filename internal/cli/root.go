// Package cli provides the command-line presentation for Okub.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mozebaltyk/Okub/internal/adapters/filesystem"
	"github.com/mozebaltyk/Okub/internal/application"
	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/spf13/cobra"
)

const (
	exitValidation   = 1
	exitInvalidUsage = 2
	exitFilesystem   = 5
)

type commandError struct {
	code  int
	err   error
	quiet bool
}

func (e *commandError) Error() string { return e.err.Error() }

// Execute runs the Okub CLI and returns a process exit code.
func Execute(args []string, version string, stdout, stderr io.Writer) int {
	cmd := newRootCommand(version)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.Execute(); err != nil {
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

func newRootCommand(version string) *cobra.Command {
	var configPath string
	var jsonOutput bool
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
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit one JSON result")

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
			cacheRoot, err := os.UserCacheDir()
			if err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("find user cache directory: %w", err)}
			}
			homeDirectory, err := os.UserHomeDir()
			if err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("find user home directory: %w", err)}
			}
			workflow := application.ConfigurationValidator{
				Source: filesystem.ConfigurationSource{}, CacheDirectory: filepath.Join(cacheRoot, "okub"), HomeDirectory: homeDirectory,
			}
			result, err := workflow.Validate(configPath)
			if err != nil {
				return &commandError{code: exitFilesystem, err: err}
			}
			if err := writeReport(cmd.OutOrStdout(), result.Report, jsonOutput); err != nil {
				return &commandError{code: exitFilesystem, err: fmt.Errorf("write result: %w", err)}
			}
			if !result.Report.Valid {
				code := exitValidation
				for _, finding := range result.Report.Findings {
					if finding.ID == "apiVersion.unsupported" {
						code = exitInvalidUsage
						break
					}
				}
				return &commandError{code: code, err: errors.New("configuration is invalid"), quiet: true}
			}
			return nil
		},
	})
	root.AddCommand(config)

	return root
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
