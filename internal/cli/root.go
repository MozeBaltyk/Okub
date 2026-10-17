// Package cli provides the command-line presentation for Okub.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const exitInvalidUsage = 2

// Execute runs the Okub CLI and returns a process exit code.
func Execute(args []string, version string, stdout, stderr io.Writer) int {
	cmd := newRootCommand(version)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(stderr, "okub: %v\n", err)
		return exitInvalidUsage
	}
	return 0
}

func newRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "okub",
		Short:         "Prepare, validate, and bootstrap bare-metal OKD clusters",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate("okub {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the Okub version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "okub %s\n", version)
		},
	})

	return root
}
