package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/skillpack"
)

func newSkillCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Package skills for MDM delivery via org-plugins/",
		Long: "Cowork's org-plugins/ mount folder only loads plugin bundles.\n" +
			"`skill pack` wraps a directory of skills in a minimal plugin bundle\n" +
			"so a company skill library can be delivered through the same MDM\n" +
			"channel used for regular plugins.",
	}
	cmd.AddCommand(newSkillPackCommand(stdout, stderr))
	return cmd
}

func newSkillPackCommand(stdout, _ io.Writer) *cobra.Command {
	var (
		name        string
		out         string
		version     string
		description string
		authorName  string
		authorEmail string
		force       bool
	)
	c := &cobra.Command{
		Use:   "pack INPUT_DIR",
		Short: "Wrap a skills directory in a plugin bundle for org-plugins/",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if out == "" {
				return fmt.Errorf("--out is required")
			}
			res, err := skillpack.Pack(args[0], out, skillpack.Options{
				Name:        name,
				Version:     version,
				Description: description,
				AuthorName:  authorName,
				AuthorEmail: authorEmail,
				Force:       force,
			})
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			for _, s := range res.Skills {
				fmt.Fprintf(stdout, "  - %s — %s\n", s.Name, s.Description)
			}
			fmt.Fprintf(stdout, "Packed %d skill(s) into %s\n", len(res.Skills), res.BundleDir)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "plugin bundle name (required; lowercase-kebab)")
	c.Flags().StringVar(&out, "out", "", "output bundle directory (required)")
	c.Flags().StringVar(&version, "version", "0.1.0", "plugin version written into plugin.json")
	c.Flags().StringVar(&description, "description", "", "plugin description (defaults to a generic placeholder)")
	c.Flags().StringVar(&authorName, "author", "", "optional author name")
	c.Flags().StringVar(&authorEmail, "author-email", "", "optional author email (requires --author)")
	c.Flags().BoolVar(&force, "force", false, "overwrite --out if it already exists")
	_ = c.MarkFlagRequired("name")
	_ = c.MarkFlagRequired("out")
	return c
}
