package profile

import (
	"fmt"
	"regexp"
)

// PlaceholderFinding describes one leftover REPLACE_* placeholder
// residual discovered in a Profile's values. Returned by
// LintPlaceholders as a pre-distribution gate companion to Validate
// (which is schema-only and does not catch placeholders).
//
// Only the key and the literal placeholder token are surfaced —
// surrounding value text is intentionally omitted so that lint output
// is safe to print to CI logs even when the value contains secrets.
type PlaceholderFinding struct {
	Key   string `json:"key"`
	Match string `json:"match"`
}

// placeholderPattern matches the reserved REPLACE_<ALLCAPS_DIGITS_UNDERSCORES>
// convention used by the CN-focused built-in templates
// (REPLACE_WITH_YOUR_API_KEY, REPLACE_WITH_VENDOR_BASE_URL,
// REPLACE_WITH_MODEL_ID_1, REPLACE_ME, REPLACE_WITH_YOUR_INTERNAL_DOMAIN).
// Narrow by design: older templates (bedrock-basic, vertex) use
// ACCOUNT / PROFILE_ID style variables that are documented as
// intentional slots, not scaffolded placeholders, and are deliberately
// NOT flagged.
var placeholderPattern = regexp.MustCompile(`\bREPLACE_[A-Z0-9_]+\b`)

// LintPlaceholders walks every value in p (including nested strings
// inside stringArray / jsonString values) and returns every distinct
// REPLACE_* match it finds, tagged with the top-level key that owns
// the value. A clean Profile returns nil.
func LintPlaceholders(p *Profile) []PlaceholderFinding {
	var findings []PlaceholderFinding
	for _, e := range p.Entries() {
		for _, m := range scanValue(e.Value) {
			findings = append(findings, PlaceholderFinding{Key: e.Key, Match: m})
		}
	}
	return findings
}

// scanValue returns every REPLACE_* match found inside v, recursively
// descending into string slices and map values. Non-string leaves
// (int, bool, float) cannot contain placeholders and are ignored.
func scanValue(v any) []string {
	switch t := v.(type) {
	case string:
		return placeholderPattern.FindAllString(t, -1)
	case []string:
		var out []string
		for _, s := range t {
			out = append(out, placeholderPattern.FindAllString(s, -1)...)
		}
		return out
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, scanValue(item)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, item := range t {
			out = append(out, scanValue(item)...)
		}
		return out
	default:
		return nil
	}
}

// FormatFindings returns a human-readable summary of findings suitable
// for printing to stdout from the CLI.
func FormatFindings(findings []PlaceholderFinding) string {
	if len(findings) == 0 {
		return "no placeholder residuals"
	}
	out := fmt.Sprintf("%d placeholder(s) found — fill in before distributing:\n", len(findings))
	for _, f := range findings {
		out += fmt.Sprintf("  %s: %s\n", f.Key, f.Match)
	}
	return out
}
