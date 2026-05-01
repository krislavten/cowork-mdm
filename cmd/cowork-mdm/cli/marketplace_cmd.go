package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/marketplace"
	"github.com/krislavten/cowork-mdm/internal/paths"
)

func newMarketplaceCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage git-backed plugin marketplaces (macOS)",
		Long: "Clone marketplace repositories under Claude Desktop's org-plugins/\n" +
			"directory, list them, pull updates, and rebuild top-level symlinks\n" +
			"so Claude Desktop sees every plugin.",
	}
	cmd.AddCommand(newMarketplaceAddCommand(stdout, stderr))
	cmd.AddCommand(newMarketplaceListCommand(stdout, stderr))
	cmd.AddCommand(newMarketplaceUpdateCommand(stdout, stderr))
	cmd.AddCommand(newMarketplaceRemoveCommand(stdout, stderr))
	cmd.AddCommand(newMarketplaceLinkCommand(stdout, stderr))
	return cmd
}

func managerForCLI() *marketplace.Manager {
	return marketplace.NewManager(paths.Default().OrgPluginsDir())
}

func newMarketplaceAddCommand(stdout, stderr io.Writer) *cobra.Command {
	var name string
	var depth int
	c := &cobra.Command{
		Use:   "add URL",
		Short: "Clone a marketplace repo + symlink its plugins",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := managerForCLI()
			repo, err := m.Add(context.Background(), args[0], marketplace.AddOptions{Name: name, Depth: depth})
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "cloned %s → %s (%d plugins discovered)\n", repo.Name, repo.Path, len(repo.Plugins))
			report, err := m.LinkAll()
			if err != nil {
				return err
			}
			if len(report.Created) > 0 {
				fmt.Fprintf(stdout, "created symlinks: %v\n", report.Created)
			}
			if len(report.Updated) > 0 {
				fmt.Fprintf(stdout, "updated symlinks: %v\n", report.Updated)
			}
			for _, conflict := range report.Conflicts {
				fmt.Fprintf(stderr, "conflict: %s — %s\n", conflict.Name, conflict.Reason)
			}
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "override the default repo basename")
	c.Flags().IntVar(&depth, "depth", 1, "git clone depth; pass -1 for full history")
	return c
}

func newMarketplaceListCommand(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed marketplace repos",
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := managerForCLI()
			repos, err := m.List()
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(repos)
			}
			if len(repos) == 0 {
				fmt.Fprintln(stdout, "no marketplaces installed")
				return nil
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tURL\tHEAD\tPLUGINS\tLAST-PULL")
			for _, r := range repos {
				lp := "(never)"
				if !r.LastPull.IsZero() {
					lp = r.LastPull.Format("2006-01-02 15:04")
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", r.Name, r.URL, r.CurrentRef, len(r.Plugins), lp)
			}
			return tw.Flush()
		},
	}
}

func newMarketplaceUpdateCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "update [NAME]",
		Short: "git pull marketplace repos + rebuild symlinks",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(_ *cobra.Command, args []string) error {
			m := managerForCLI()
			ctx := context.Background()
			if len(args) == 1 {
				if err := m.Update(ctx, args[0]); err != nil {
					return err
				}
				fmt.Fprintf(stdout, "updated %s\n", args[0])
			} else {
				results := m.UpdateAll(ctx)
				failed := 0
				for _, r := range results {
					if r.Err != nil {
						fmt.Fprintf(stderr, "%s: %v\n", r.Name, r.Err)
						failed++
						continue
					}
					if r.Updated {
						fmt.Fprintf(stdout, "%s: %s → %s\n", r.Name, r.FromRef, r.ToRef)
					} else {
						fmt.Fprintf(stdout, "%s: up to date\n", r.Name)
					}
				}
				if failed > 0 {
					return fmt.Errorf("%d marketplace(s) failed to update", failed)
				}
			}
			report, err := m.LinkAll()
			if err != nil {
				return err
			}
			for _, name := range report.Created {
				fmt.Fprintf(stdout, "linked new: %s\n", name)
			}
			for _, name := range report.Updated {
				fmt.Fprintf(stdout, "retargeted: %s\n", name)
			}
			for _, c := range report.Conflicts {
				fmt.Fprintf(stderr, "conflict: %s — %s\n", c.Name, c.Reason)
			}
			return nil
		},
	}
}

func newMarketplaceRemoveCommand(stdout, stderr io.Writer) *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a marketplace + associated symlinks",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes {
				fmt.Fprintf(stderr, "marketplace remove %s is destructive — pass --yes to confirm\n", args[0])
				return fmt.Errorf("confirmation required")
			}
			m := managerForCLI()
			if err := m.Remove(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "removed %s\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "confirm destructive removal")
	return c
}

func newMarketplaceLinkCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "link",
		Short: "Rebuild top-level symlinks from all installed marketplaces",
		RunE: func(_ *cobra.Command, _ []string) error {
			m := managerForCLI()
			report, err := m.LinkAll()
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "created=%d updated=%d unchanged=%d\n",
				len(report.Created), len(report.Updated), len(report.Unchanged))
			for _, c := range report.Conflicts {
				fmt.Fprintf(stderr, "conflict: %s — %s\n", c.Name, c.Reason)
			}
			return nil
		},
	}
}
