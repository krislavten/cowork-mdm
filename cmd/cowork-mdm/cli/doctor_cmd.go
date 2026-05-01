package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/doctor"
)

func newDoctorCommand(stdout, stderr io.Writer) *cobra.Command {
	var fix bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Run environment health checks",
		Long: "Enumerates host state: Claude Desktop install, managed plist/registry,\n" +
			"org-plugins/ layout, symlink health, and per-user sessions. Returns\n" +
			"non-zero exit on any error-level check.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r := doctor.DefaultRunner()
			ctx := context.Background()
			var results []doctor.Result
			if fix {
				results = r.RunAndFix(ctx)
			} else {
				results = r.Run(ctx)
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				b, err := doctor.JSON(results)
				if err != nil {
					return err
				}
				_, _ = stdout.Write(b)
				_, _ = stdout.Write([]byte{'\n'})
			} else {
				for _, r := range results {
					tag := statusSymbol(r.Status)
					fmt.Fprintf(stdout, "%s %-30s %s\n", tag, r.ID, r.Message)
					if r.Detail != "" {
						fmt.Fprintf(stdout, "    %s\n", r.Detail)
					}
				}
				// Summary
				counts := map[doctor.Status]int{}
				for _, r := range results {
					counts[r.Status]++
				}
				fmt.Fprintf(stdout, "\n%d ok, %d warning, %d error, %d skipped\n",
					counts[doctor.StatusOK], counts[doctor.StatusWarning],
					counts[doctor.StatusError], counts[doctor.StatusSkipped])
			}
			if code := doctor.Exit(results); code != 0 {
				return fmt.Errorf("doctor reported errors")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&fix, "fix", false, "attempt auto-fix for checks that support it")
	return c
}

func statusSymbol(s doctor.Status) string {
	switch s {
	case doctor.StatusOK:
		return "✓"
	case doctor.StatusWarning:
		return "!"
	case doctor.StatusError:
		return "✗"
	case doctor.StatusSkipped:
		return "-"
	}
	return "?"
}
