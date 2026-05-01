package profile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

// MobileConfigOpts controls how a profile is wrapped into an Apple
// Configuration Profile (.mobileconfig). PayloadUUIDs are randomized unless
// explicitly set, which is how Apple expects it — UUIDs should change across
// exports to avoid MDM collisions. Tests supply fixed values.
type MobileConfigOpts struct {
	PayloadIdentifier string // default "com.yuanli.cowork-mdm.<slug-of-name>"

	// PayloadUUID is the inner payload's UUID (the one for the
	// com.anthropic.claudefordesktop payload entry). This matches the
	// spec's naming — if you only set one UUID this is the one you'd
	// usually want to pin.
	PayloadUUID string // default random v4-shaped

	// ConfigurationPayloadUUID is the outer Configuration envelope's UUID.
	// Usually fine to leave randomized; pin only for golden-file tests.
	ConfigurationPayloadUUID string // default random v4-shaped

	PayloadOrganization string // default empty
	PayloadScope        string // "System" (default) | "User"
}

// EncodeMobileConfig wraps the profile's MDM key/value pairs in the standard
// Apple Configuration Profile XML structure.
//
// Critical encoding rule (mirrors Claude.app's jee() reader): arrays and
// jsonString values go inside <string> elements as JSON text, NOT as <array>.
// Claude.app calls JSON.parse on the string — a native plist array fails.
func EncodeMobileConfig(p *Profile, opts MobileConfigOpts) ([]byte, error) {
	if opts.PayloadIdentifier == "" {
		opts.PayloadIdentifier = "com.yuanli.cowork-mdm." + slugify(p.Name)
	}
	if opts.ConfigurationPayloadUUID == "" {
		opts.ConfigurationPayloadUUID = randomUUID()
	}
	if opts.PayloadUUID == "" {
		opts.PayloadUUID = randomUUID()
	}
	if opts.PayloadScope == "" {
		opts.PayloadScope = "System"
	}

	// Validate up-front so callers never receive a mobileconfig with known-
	// bad MDM values. This includes schema-known unknown-key errors.
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("profile validation failed: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(plistHeader)
	buf.WriteString("<plist version=\"1.0\">\n")
	buf.WriteString("<dict>\n")

	fmt.Fprintf(&buf, "\t<key>PayloadContent</key>\n")
	fmt.Fprintln(&buf, "\t<array>")
	fmt.Fprintln(&buf, "\t\t<dict>")
	writeKV(&buf, "\t\t\t", "PayloadType", "com.anthropic.claudefordesktop")
	writeKV(&buf, "\t\t\t", "PayloadIdentifier", opts.PayloadIdentifier+".settings")
	writeKV(&buf, "\t\t\t", "PayloadUUID", opts.PayloadUUID)
	writeKV(&buf, "\t\t\t", "PayloadDisplayName", p.Name)
	fmt.Fprintln(&buf, "\t\t\t<key>PayloadVersion</key>")
	fmt.Fprintln(&buf, "\t\t\t<integer>1</integer>")
	if err := writeProfileEntries(&buf, p, "\t\t\t"); err != nil {
		return nil, fmt.Errorf("profile encode failed: %w", err)
	}
	fmt.Fprintln(&buf, "\t\t</dict>")
	fmt.Fprintln(&buf, "\t</array>")
	writeKV(&buf, "\t", "PayloadDisplayName", p.Name)
	writeKV(&buf, "\t", "PayloadIdentifier", opts.PayloadIdentifier)
	if opts.PayloadOrganization != "" {
		writeKV(&buf, "\t", "PayloadOrganization", opts.PayloadOrganization)
	}
	writeKV(&buf, "\t", "PayloadType", "Configuration")
	writeKV(&buf, "\t", "PayloadUUID", opts.ConfigurationPayloadUUID)
	fmt.Fprintln(&buf, "\t<key>PayloadVersion</key>")
	fmt.Fprintln(&buf, "\t<integer>1</integer>")
	writeKV(&buf, "\t", "PayloadScope", opts.PayloadScope)
	buf.WriteString("</dict>\n")
	buf.WriteString("</plist>\n")
	return buf.Bytes(), nil
}

const plistHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
`

// writeProfileEntries emits <key>...</key><value/> pairs for every entry in
// the profile using the plist type mapping specified in specs/profile.md.
func writeProfileEntries(w io.Writer, p *Profile, indent string) error {
	s := schema.Load()
	for _, e := range p.entries {
		k := s.Find(e.Key)
		if k == nil {
			return &UnknownKeyError{Key: e.Key}
		}
		if err := writePlistValue(w, indent, e.Key, k.Type, e.Value); err != nil {
			return fmt.Errorf("%s: %w", e.Key, err)
		}
	}
	return nil
}

// writePlistValue writes one <key>+<value> pair per the type mapping.
func writePlistValue(w io.Writer, indent, key string, t schema.Type, value any) error {
	fmt.Fprintf(w, "%s<key>%s</key>\n", indent, escapeXML(key))

	switch t {
	case schema.TypeBoolean:
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("boolean value expected, got %T", value)
		}
		if b {
			fmt.Fprintf(w, "%s<true/>\n", indent)
		} else {
			fmt.Fprintf(w, "%s<false/>\n", indent)
		}

	case schema.TypeInteger:
		var n int64
		switch v := value.(type) {
		case int:
			n = int64(v)
		case int64:
			n = v
		case int32:
			n = int64(v)
		case float64:
			if v != float64(int64(v)) {
				return fmt.Errorf("integer value expected, got non-whole float %v", v)
			}
			n = int64(v)
		default:
			return fmt.Errorf("integer value expected, got %T", value)
		}
		fmt.Fprintf(w, "%s<integer>%d</integer>\n", indent, n)

	case schema.TypeString, schema.TypeURL, schema.TypeEnum:
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("string value expected, got %T", value)
		}
		fmt.Fprintf(w, "%s<string>%s</string>\n", indent, escapeXML(s))

	case schema.TypeStringArray:
		// Arrays go as JSON strings per the managed-prefs reader contract.
		arr, err := coerceStringArray(value)
		if err != nil {
			return err
		}
		b, err := json.Marshal(arr)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s<string>%s</string>\n", indent, escapeXML(string(b)))

	case schema.TypeJSONString:
		// jsonString values are stored as a string; accept either a raw JSON
		// string or a pre-unmarshaled map/slice and marshal it deterministically.
		s, err := coerceJSONString(value)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s<string>%s</string>\n", indent, escapeXML(s))

	default:
		return fmt.Errorf("unsupported schema type %q", t)
	}
	return nil
}

func coerceStringArray(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("stringArray element %d is %T, want string", i, item)
			}
			out[i] = s
		}
		return out, nil
	default:
		return nil, fmt.Errorf("stringArray value expected, got %T", value)
	}
}

func coerceJSONString(value any) (string, error) {
	if s, ok := value.(string); ok {
		// Validate it parses as JSON so we don't smuggle garbage through.
		var any any
		if err := json.Unmarshal([]byte(s), &any); err != nil {
			return "", fmt.Errorf("jsonString is not valid JSON: %w", err)
		}
		return s, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("cannot marshal jsonString value: %w", err)
	}
	return string(b), nil
}

func writeKV(w io.Writer, indent, key, value string) {
	fmt.Fprintf(w, "%s<key>%s</key>\n", indent, escapeXML(key))
	fmt.Fprintf(w, "%s<string>%s</string>\n", indent, escapeXML(value))
}

// escapeXML escapes the five reserved XML characters. Values may contain
// characters that matter inside plist text content — we can't rely on
// encoding/xml because we emit hand-formatted plist for golden-file
// stability.
func escapeXML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// slugify produces a simple ASCII-slug for use in PayloadIdentifier suffixes.
func slugify(s string) string {
	if s == "" {
		return "profile"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ', r == '.':
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out == "" {
		return "profile"
	}
	return out
}

// randomUUID returns a v4-shaped UUID string (random bytes formatted as
// xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx). Apple's tools don't verify the
// RFC-4122 variant bits but we set them anyway for wellformedness.
//
// If crypto/rand fails (OS denying entropy — very rare), we return a
// documented all-zeros sentinel so output is still well-formed XML. Callers
// that care should pass explicit UUIDs via MobileConfigOpts.
func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return strings.ToUpper(s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32])
}
