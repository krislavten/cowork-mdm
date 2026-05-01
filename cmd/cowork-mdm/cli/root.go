// Package cli wires the cowork-mdm command-line interface.
//
// v0.1 surface:
//
//	cowork-mdm schema list      — list all 51 MDM keys
//	cowork-mdm schema show KEY  — show one key's full detail
//	cowork-mdm paths show       — show resolved host paths
//	cowork-mdm --version        — print version
//
// More commands (profile, marketplace, plugin, doctor) arrive in v0.2.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// BuildInfo carries release metadata populated via -ldflags by GoReleaser.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Execute runs the CLI and returns the appropriate exit code.
//
//	0 — success
//	1 — operation failed (validation / check / runtime)
//	2 — argument / flag error (set by cobra)
func Execute(info BuildInfo) int {
	root := NewRootCommand(info, os.Stdout, os.Stderr)
	if err := root.Execute(); err != nil {
		// cobra already printed the message; return non-zero.
		return 1
	}
	return 0
}

// NewRootCommand builds the root cobra command. Separated from Execute so
// tests can construct it, redirect its streams, and drive subcommands.
func NewRootCommand(info BuildInfo, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "cowork-mdm",
		Short: "Claude Desktop enterprise deployment toolkit",
		Long: "cowork-mdm generates and inspects Managed Preferences (MDM) configuration\n" +
			"for Claude Desktop. v0.1 ships schema and path references; v0.2 adds\n" +
			"profile generation, marketplace management, and diagnostics.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       fmt.Sprintf("%s (commit %s, built %s)", info.Version, info.Commit, info.Date),
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().Bool("json", false, "machine-readable JSON output (where supported)")
	root.PersistentFlags().Bool("no-color", false, "disable ANSI color in output")

	root.AddCommand(newSchemaCommand(stdout, stderr))
	root.AddCommand(newPathsCommand(stdout, stderr))

	return root
}
