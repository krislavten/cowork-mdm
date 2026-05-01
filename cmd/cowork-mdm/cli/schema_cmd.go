package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

func newSchemaCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Inspect the embedded MDM key schema",
	}
	cmd.AddCommand(newSchemaListCommand(stdout, stderr))
	cmd.AddCommand(newSchemaShowCommand(stdout, stderr))
	return cmd
}

func newSchemaListCommand(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all MDM keys",
		Long:  "Prints every key defined in the embedded schema as a table. Use --json for machine-readable output.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := schema.Load()
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(s.Keys)
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tSCOPES\tAPPMIN\tTITLE")
			for i := range s.Keys {
				k := &s.Keys[i]
				scopes := scopesToString(k.Scopes)
				title := k.Title
				if len(title) > 60 {
					title = title[:57] + "..."
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					k.Name, k.Type, scopes, defaultDash(k.AppMin), title)
			}
			_ = tw.Flush()
			fmt.Fprintf(stdout, "\n%d keys total (extracted from Claude.app %s)\n",
				len(s.Keys), s.ExtractedFromAppVersion)
			return nil
		},
	}
}

func newSchemaShowCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "show KEY",
		Short: "Show one key's full detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := schema.Load()
			k := s.Find(args[0])
			if k == nil {
				fmt.Fprintf(stderr, "schema show: unknown key %q\n", args[0])
				return fmt.Errorf("unknown key")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(k)
			}
			fmt.Fprintf(stdout, "Name:         %s\n", k.Name)
			fmt.Fprintf(stdout, "Type:         %s\n", k.Type)
			fmt.Fprintf(stdout, "Scopes:       %s\n", scopesToString(k.Scopes))
			if k.AppMin != "" {
				fmt.Fprintf(stdout, "AppMin:       %s\n", k.AppMin)
			}
			if k.Title != "" {
				fmt.Fprintf(stdout, "Title:        %s\n", k.Title)
			}
			if k.Provider != "" {
				fmt.Fprintf(stdout, "Provider:     %s\n", k.Provider)
			}
			if k.Category != "" {
				fmt.Fprintf(stdout, "Category:     %s\n", k.Category)
			}
			if k.Sensitive {
				fmt.Fprintln(stdout, "Sensitive:    true (values hidden in logs)")
			}
			if k.LegacyAlias != "" {
				fmt.Fprintf(stdout, "LegacyAlias:  %s\n", k.LegacyAlias)
			}
			if len(k.EnumValues) > 0 {
				fmt.Fprintf(stdout, "Allowed:      %s\n", strings.Join(k.EnumValues, " | "))
			}
			if k.Default != nil {
				fmt.Fprintf(stdout, "Default:      %v\n", k.Default)
			}
			if k.Example != nil {
				fmt.Fprintf(stdout, "Example:      %v\n", k.Example)
			}
			if k.Description != "" {
				fmt.Fprintln(stdout, "\nDescription:")
				fmt.Fprintln(stdout, indent(k.Description, "  "))
			}
			return nil
		},
	}
}

func scopesToString(scopes []schema.Scope) string {
	if len(scopes) == 0 {
		return "-"
	}
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}

func defaultDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func indent(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
