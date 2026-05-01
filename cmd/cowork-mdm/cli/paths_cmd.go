package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/paths"
)

func newPathsCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "paths",
		Short: "Show resolved host paths",
	}
	cmd.AddCommand(newPathsShowCommand(stdout, stderr))
	return cmd
}

func newPathsShowCommand(stdout, _ io.Writer) *cobra.Command {
	var osFlag string
	c := &cobra.Command{
		Use:   "show",
		Short: "Print the paths cowork-mdm would read on the host",
		Long: "Shows the managed plist location, org-plugins dir, user sessions dir,\n" +
			"and related paths. Pass --os to simulate another platform.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var p paths.Provider
			if osFlag == "" || osFlag == "host" {
				p = paths.Default()
			} else {
				p = paths.ForOS(osFlag)
			}
			entries := []struct {
				name string
				val  string
			}{
				{"ClaudeAppPath", p.ClaudeAppPath()},
				{"ManagedPrefsPlist", p.ManagedPrefsPlist()},
				{"ManagedPrefsUserPlist(<you>)", p.ManagedPrefsUserPlist(currentUsername())},
				{"OrgPluginsDir", p.OrgPluginsDir()},
				{"UserSessionsDir", p.UserSessionsDir()},
				{"LaunchAgentDir", p.LaunchAgentDir()},
				{"WindowsRegistryPath", p.WindowsRegistryPath()},
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				out := make(map[string]string, len(entries))
				for _, e := range entries {
					out[e.name] = e.val
				}
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tVALUE")
			for _, e := range entries {
				v := e.val
				if v == "" {
					v = "(not applicable on this OS)"
				}
				fmt.Fprintf(tw, "%s\t%s\n", e.name, v)
			}
			return tw.Flush()
		},
	}
	c.Flags().StringVar(&osFlag, "os", "", `simulate a different OS: "darwin", "windows", "linux"`)
	return c
}

func currentUsername() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "user"
}
