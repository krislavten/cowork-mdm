package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/krislavten/cowork-mdm/internal/managed"
	"github.com/krislavten/cowork-mdm/internal/profile"
)

func newProfileCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Generate, validate, and apply MDM profiles",
	}
	cmd.AddCommand(newProfileNewCommand(stdout, stderr))
	cmd.AddCommand(newProfileValidateCommand(stdout, stderr))
	cmd.AddCommand(newProfileLintCommand(stdout, stderr))
	cmd.AddCommand(newProfileApplyCommand(stdout, stderr))
	cmd.AddCommand(newProfileStatusCommand(stdout, stderr))
	cmd.AddCommand(newProfileTemplatesCommand(stdout, stderr))
	cmd.AddCommand(newProfileShowTemplateCommand(stdout, stderr))
	return cmd
}

// --- profile new ---

func newProfileNewCommand(stdout, stderr io.Writer) *cobra.Command {
	var (
		template        string
		fromFile        string
		outFile         string
		format          string
		payloadIDPrefix string
		setFlags        []string
	)
	c := &cobra.Command{
		Use:   "new",
		Short: "Generate a new MDM profile from a template or YAML file",
		Long: "Loads a built-in template (--template) or a user-supplied YAML file\n" +
			"(--from), applies any --set KEY=VALUE overrides, and emits the profile\n" +
			"in the requested --format. Default format is mobileconfig.\n\n" +
			"Use --payload-identifier-prefix to stamp your org's reverse-DNS onto\n" +
			"the mobileconfig's PayloadIdentifier. Precedence: flag > env var\n" +
			"COWORK_MDM_PAYLOAD_ID_PREFIX > default (com.cowork-mdm).\n\n" +
			"Keep enterprise-specific values (ARNs, MCP tokens) in your own YAML file\n" +
			"using --from; never commit those to the template directory.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := loadSourceProfile(template, fromFile)
			if err != nil {
				return err
			}
			for _, set := range setFlags {
				key, val, err := parseSet(set)
				if err != nil {
					return err
				}
				if err := setFromString(p, key, val); err != nil {
					return fmt.Errorf("--set %s: %w", set, err)
				}
			}
			data, err := encodeProfile(p, format, payloadIDPrefix)
			if err != nil {
				return err
			}
			if outFile == "" || outFile == "-" {
				_, err := stdout.Write(data)
				return err
			}
			if err := os.WriteFile(outFile, data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", outFile, err)
			}
			fmt.Fprintf(stderr, "wrote %d bytes to %s\n", len(data), outFile)
			return nil
		},
	}
	c.Flags().StringVarP(&template, "template", "t", "", "built-in template name (e.g. bedrock-basic). Use `profile templates` to list.")
	c.Flags().StringVar(&fromFile, "from", "", "path to a YAML file describing the profile")
	c.Flags().StringVarP(&outFile, "out", "o", "", "output file (default stdout)")
	c.Flags().StringVarP(&format, "format", "f", "mobileconfig", "output format: mobileconfig | plist")
	c.Flags().StringVar(&payloadIDPrefix, "payload-identifier-prefix", "", "reverse-DNS prefix for PayloadIdentifier (e.g. com.acme.it). Overrides $COWORK_MDM_PAYLOAD_ID_PREFIX and the default com.cowork-mdm.")
	c.Flags().StringArrayVar(&setFlags, "set", nil, "override a key: --set KEY=VALUE (repeatable)")
	return c
}

// --- profile validate ---

func newProfileValidateCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "validate FILE",
		Short: "Validate a .mobileconfig or .plist file against the schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			p, report, err := decodeByDetection(data)
			if err != nil {
				return err
			}
			valid := true
			if err := p.Validate(); err != nil {
				valid = false
				fmt.Fprintf(stdout, "validation errors:\n")
				fmt.Fprintf(stdout, "  %s\n", strings.ReplaceAll(err.Error(), "\n", "\n  "))
			}
			if len(report.UnknownKeys) > 0 {
				fmt.Fprintf(stdout, "unknown keys (not in schema):\n")
				for _, uk := range report.UnknownKeys {
					fmt.Fprintf(stdout, "  %s\n", uk.Key)
				}
			}
			if valid && len(report.UnknownKeys) == 0 {
				fmt.Fprintf(stdout, "%s: OK (%d keys)\n", args[0], p.Len())
				return nil
			}
			fmt.Fprintf(stderr, "%s: issues found\n", args[0])
			return fmt.Errorf("validation failed")
		},
	}
}

// --- profile apply ---

func newProfileApplyCommand(stdout, stderr io.Writer) *cobra.Command {
	var (
		dryRun bool
		hive   string
	)
	c := &cobra.Command{
		Use:   "apply FILE",
		Short: "Apply a profile to the host's managed-prefs store (requires root/admin)",
		Long: "Reads FILE, decodes it into a Profile, and writes it to the host's\n" +
			"managed-prefs location (/Library/Managed Preferences/ on macOS,\n" +
			"HKLM\\SOFTWARE\\Policies\\Claude on Windows by default).\n\n" +
			"Use --dry-run to preview what would be written without touching disk.\n" +
			"On macOS this warns that direct writes bypass MDM resync; for production\n" +
			"deployments push the .mobileconfig through your MDM channel instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			p, _, err := decodeByDetection(data)
			if err != nil {
				return err
			}
			res, err := managed.Apply(p, managed.ApplyOptions{
				DryRun: dryRun,
				Hive:   hive,
			})
			if err != nil {
				if errors.Is(err, managed.ErrPermission) {
					fmt.Fprintln(stderr, "permission denied — re-run with sudo (macOS) or as admin (Windows)")
				}
				return err
			}
			if res.DryRun {
				fmt.Fprintf(stdout, "(dry-run) would write %d bytes to %s\n\n", len(res.Preview), res.TargetPath)
				fmt.Fprintln(stdout, res.Preview)
				return nil
			}
			fmt.Fprintf(stdout, "wrote %d bytes to %s (%s)\n", res.BytesWritten, res.TargetPath, res.Platform)
			for _, w := range res.Warnings {
				fmt.Fprintf(stderr, "warning: %s\n", w)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be written; don't touch disk/registry")
	c.Flags().StringVar(&hive, "hive", "", "Windows only: HKLM (default) or HKCU")
	return c
}

// --- profile status ---

func newProfileStatusCommand(stdout, stderr io.Writer) *cobra.Command {
	var (
		hive     string
		source   string
		unmasked bool
	)
	c := &cobra.Command{
		Use:   "status",
		Short: "Show the currently applied managed profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := managed.Status(managed.StatusOptions{Hive: hive, SourcePath: source})
			if err != nil {
				if errors.Is(err, managed.ErrUnsupportedPlatform) {
					fmt.Fprintln(stderr, "status: unsupported on this platform")
				}
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				// Build a plain payload so we don't depend on Profile's unexported fields.
				var out any
				type profPayload struct {
					Name   string         `json:"name"`
					Values map[string]any `json:"values"`
				}
				if rep.Profile != nil {
					pp := profPayload{Name: rep.Profile.Name, Values: map[string]any{}}
					for _, k := range rep.Profile.Keys() {
						v, _ := rep.Profile.Get(k)
						pp.Values[k] = v
					}
					out = map[string]any{
						"platform":    rep.Platform,
						"targetPath":  rep.TargetPath,
						"present":     rep.Present,
						"profile":     pp,
						"unknownKeys": rep.UnknownKeys,
						"parseError":  rep.ParseError,
					}
				} else {
					out = map[string]any{
						"platform":   rep.Platform,
						"targetPath": rep.TargetPath,
						"present":    rep.Present,
						"parseError": rep.ParseError,
					}
				}
				return enc.Encode(out)
			}
			fmt.Fprintf(stdout, "Platform:    %s\n", rep.Platform)
			fmt.Fprintf(stdout, "Target:      %s\n", rep.TargetPath)
			if !rep.Present {
				fmt.Fprintln(stdout, "Status:      no managed profile installed")
				return nil
			}
			if rep.ParseError != "" {
				fmt.Fprintf(stdout, "Parse error: %s\n", rep.ParseError)
				return nil
			}
			fmt.Fprintf(stdout, "Keys:        %d\n", rep.Profile.Len())
			if len(rep.UnknownKeys) > 0 {
				fmt.Fprintln(stdout, "Unknown keys (not in current schema):")
				for _, uk := range rep.UnknownKeys {
					fmt.Fprintf(stdout, "  %s\n", uk.Key)
				}
			}
			fmt.Fprintln(stdout, "\nConfigured values:")
			for _, k := range rep.Profile.Keys() {
				v, _ := rep.Profile.Get(k)
				fmt.Fprintf(stdout, "  %-40s %s\n", k, formatStatusValue(k, v, unmasked))
			}
			return nil
		},
	}
	c.Flags().StringVar(&hive, "hive", "", "Windows only: HKLM (default) or HKCU")
	c.Flags().StringVar(&source, "source", "", "read this file instead of the default managed-prefs location (accepts .mobileconfig or .plist)")
	c.Flags().BoolVar(&unmasked, "unmasked", false, "show raw sensitive values in human-readable output (default: redact)")
	return c
}

// --- profile templates ---

func newProfileTemplatesCommand(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "templates",
		Short: "List built-in profile templates",
		RunE: func(_ *cobra.Command, _ []string) error {
			for _, n := range profile.TemplateNames() {
				fmt.Fprintln(stdout, n)
			}
			return nil
		},
	}
}

// --- profile show-template ---

func newProfileShowTemplateCommand(stdout, _ io.Writer) *cobra.Command {
	var outFile string
	c := &cobra.Command{
		Use:   "show-template NAME",
		Short: "Dump the YAML source of a built-in template",
		Long: `Dump the raw YAML source of a built-in profile template to stdout
(or to a file via --out). Useful as a starting point for an enterprise
overrides YAML, avoiding the need to curl from GitHub:

  cowork-mdm profile show-template enterprise-cn-full --out overrides.yaml

List available templates with: cowork-mdm profile templates`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := profile.ReadTemplateSource(args[0])
			if err != nil {
				return err
			}
			if outFile != "" {
				if err := os.WriteFile(outFile, data, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", outFile, err)
				}
				return nil
			}
			_, err = stdout.Write(data)
			return err
		},
	}
	c.Flags().StringVar(&outFile, "out", "", "write to this path instead of stdout")
	return c
}

// --- profile lint ---

func newProfileLintCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "lint FILE",
		Short: "Flag REPLACE_* placeholder residuals in a generated profile",
		Long: `Pre-distribution gate that scans every value in a generated
mobileconfig / plist for leftover REPLACE_* placeholder tokens (e.g.
REPLACE_WITH_YOUR_API_KEY, REPLACE_ME). Exits non-zero on any finding.

Complements profile validate, which is schema-only and does NOT catch
placeholders. Scope is narrow by design: only the REPLACE_<CAPS>
convention is flagged; older template variable slots like ACCOUNT /
PROFILE_ID in bedrock-basic are intentional and NOT flagged.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			p, _, err := decodeByDetection(data)
			if err != nil {
				return err
			}
			findings := profile.LintPlaceholders(p)
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				payload := map[string]any{
					"file":     args[0],
					"findings": findings,
				}
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(stdout, "%s: %s", args[0], profile.FormatFindings(findings))
			}
			if len(findings) > 0 {
				fmt.Fprintf(stderr, "%s: %d placeholder residual(s) present — do not distribute\n", args[0], len(findings))
				return fmt.Errorf("placeholder residuals")
			}
			return nil
		},
	}
}

// --- helpers ---

func loadSourceProfile(template, fromFile string) (*profile.Profile, error) {
	if template == "" && fromFile == "" {
		return nil, fmt.Errorf("one of --template or --from is required")
	}
	if template != "" && fromFile != "" {
		return nil, fmt.Errorf("--template and --from are mutually exclusive")
	}
	if template != "" {
		return profile.LoadTemplate(template)
	}
	data, err := os.ReadFile(fromFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fromFile, err)
	}
	return profile.LoadTemplateFile(data)
}

func parseSet(s string) (key, value string, err error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("--set requires KEY=VALUE format, got %q", s)
	}
	return strings.TrimSpace(parts[0]), parts[1], nil
}

// setFromString coerces a string value into the schema-typed Go value for
// the given key and Set()s it on the profile. Accepts:
//
//   - bool:        "true"/"false"/"1"/"0"
//   - integer:     decimal int
//   - stringArray: JSON array literal, e.g. `["a","b"]`
//   - jsonString:  raw JSON text (passed through; validated by encoder)
//   - string/url/enum: the raw string
func setFromString(p *profile.Profile, key, raw string) error {
	k := schemaLoadKey(key)
	if k == nil {
		return fmt.Errorf("unknown key %q", key)
	}
	var v any
	switch k.Type {
	case "boolean":
		switch strings.ToLower(raw) {
		case "true", "1", "yes":
			v = true
		case "false", "0", "no":
			v = false
		default:
			return fmt.Errorf("expected boolean (true/false), got %q", raw)
		}
	case "integer":
		n, err := parseInt64(raw)
		if err != nil {
			return fmt.Errorf("expected integer, got %q", raw)
		}
		v = n
	case "stringArray":
		var arr []string
		if err := jsonDecode(raw, &arr); err != nil {
			return fmt.Errorf("expected JSON array of strings, got %q: %w", raw, err)
		}
		v = arr
	default:
		// string / url / enum / jsonString: pass through
		v = raw
	}
	return p.Set(key, v)
}

// schemaLoadKey and parseInt64 / jsonDecode are tiny wrappers we need here
// to avoid widening the cli package's surface; they live in a separate
// helpers file.

func encodeProfile(p *profile.Profile, format, payloadIDPrefix string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "", "mobileconfig":
		return profile.EncodeMobileConfig(p, profile.MobileConfigOpts{
			PayloadIdentifierPrefix: payloadIDPrefix,
		})
	case "plist":
		return profile.EncodePlist(p)
	default:
		return nil, fmt.Errorf("unsupported format %q (want mobileconfig or plist)", format)
	}
}

func decodeByDetection(data []byte) (*profile.Profile, profile.DecodeReport, error) {
	switch profile.Detect(data) {
	case "mobileconfig":
		return profile.DecodeMobileConfig(data)
	case "plist":
		return profile.DecodePlist(data)
	default:
		return nil, profile.DecodeReport{}, fmt.Errorf("could not detect format (expected mobileconfig or plist)")
	}
}
