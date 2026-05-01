package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/paths"
	"github.com/krislavten/cowork-mdm/internal/plugin"
)

func newPluginCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Inspect + manage individual plugins under org-plugins/",
	}
	cmd.AddCommand(newPluginListCommand(stdout, stderr))
	cmd.AddCommand(newPluginShowCommand(stdout, stderr))
	cmd.AddCommand(newPluginUnlinkCommand(stdout, stderr))
	cmd.AddCommand(newPluginPruneCommand(stdout, stderr))
	return cmd
}

func inspectorForCLI() *plugin.Inspector {
	p := paths.Default()
	return plugin.NewInspector(p.OrgPluginsDir(), p.UserSessionsDir())
}

func mutatorForCLI() *plugin.Mutator {
	return plugin.NewMutator(paths.Default().OrgPluginsDir())
}

func newPluginListCommand(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every plugin under org-plugins/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			plugins, err := inspectorForCLI().List()
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(plugins)
			}
			if len(plugins) == 0 {
				fmt.Fprintln(stdout, "no plugins found")
				return nil
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSOURCE\tSTATUS\tVERSION")
			for _, p := range plugins {
				status := "ok"
				if p.Dangling {
					status = "dangling"
				} else if p.IsSymlink && p.Source == string(plugin.SourceSymlinkUnknown) {
					status = "external-symlink"
				}
				version := "-"
				if p.Manifest != nil && p.Manifest.Version != "" {
					version = p.Manifest.Version
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, p.Source, status, version)
			}
			return tw.Flush()
		},
	}
}

func newPluginShowCommand(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "show NAME",
		Short: "Show one plugin's full detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := inspectorForCLI().Get(args[0])
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(p)
			}
			fmt.Fprintf(stdout, "Name:       %s\n", p.Name)
			fmt.Fprintf(stdout, "Source:     %s\n", p.Source)
			fmt.Fprintf(stdout, "Target:     %s\n", p.TargetPath)
			fmt.Fprintf(stdout, "Symlink:    %t\n", p.IsSymlink)
			if p.Dangling {
				fmt.Fprintln(stdout, "Dangling:   yes")
			}
			if p.Manifest != nil {
				fmt.Fprintln(stdout, "\nManifest:")
				fmt.Fprintf(stdout, "  name:    %s\n", p.Manifest.Name)
				fmt.Fprintf(stdout, "  version: %s\n", p.Manifest.Version)
				if p.Manifest.Description != "" {
					fmt.Fprintf(stdout, "  desc:    %s\n", p.Manifest.Description)
				}
			}
			// Session enablement
			states, err := inspectorForCLI().EnabledStates()
			if err == nil {
				for _, st := range states {
					if st.Plugin == p.Name && len(st.BySession) > 0 {
						fmt.Fprintln(stdout, "\nEnabled state per session:")
						for _, s := range st.BySession {
							e := "(not configured)"
							if s.Enabled != nil {
								if *s.Enabled {
									e = "enabled"
								} else {
									e = "disabled"
								}
							}
							inst := ""
							if s.Installed {
								inst = " installed"
							}
							fmt.Fprintf(stdout, "  %s: %s%s\n", s.SessionPath, e, inst)
						}
					}
				}
			}
			return nil
		},
	}
}

func newPluginUnlinkCommand(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "unlink NAME",
		Short: "Remove a top-level symlink (does not touch real directories)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := mutatorForCLI().Unlink(args[0]); err != nil {
				if errors.Is(err, plugin.ErrPluginNotFound) {
					return fmt.Errorf("plugin %q not found", args[0])
				}
				return err
			}
			fmt.Fprintf(stdout, "unlinked %s\n", args[0])
			return nil
		},
	}
}

func newPluginPruneCommand(stdout, _ io.Writer) *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "prune",
		Short: "Remove dangling top-level symlinks (dry-run by default)",
		RunE: func(_ *cobra.Command, _ []string) error {
			plugins, err := inspectorForCLI().List()
			if err != nil {
				return err
			}
			var dangling []string
			for _, p := range plugins {
				if p.Dangling {
					dangling = append(dangling, p.Name)
				}
			}
			if len(dangling) == 0 {
				fmt.Fprintln(stdout, "no dangling symlinks")
				return nil
			}
			fmt.Fprintf(stdout, "would remove %d dangling symlink(s): %v\n", len(dangling), dangling)
			if !yes {
				fmt.Fprintln(stdout, "(dry-run — pass --yes to actually delete)")
				return nil
			}
			removed, err := mutatorForCLI().Prune()
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "removed %v\n", removed)
			return nil
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "actually delete; without this, prune is dry-run")
	return c
}
