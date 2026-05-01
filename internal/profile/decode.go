package profile

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

// Detect inspects data and returns the likely format: "mobileconfig",
// "plist", "reg", or "" (unknown).
//
// mobileconfig vs plist: both are plist XML, but mobileconfig's top-level
// dict has a PayloadContent array. We check for that marker.
func Detect(data []byte) string {
	// reg files begin with "Windows Registry Editor" magic header.
	if bytes.HasPrefix(data, []byte("Windows Registry Editor")) {
		return "reg"
	}
	if bytes.Contains(data, []byte("<!DOCTYPE plist")) || bytes.Contains(data, []byte("<plist")) {
		if bytes.Contains(data, []byte("PayloadContent")) {
			return "mobileconfig"
		}
		return "plist"
	}
	return ""
}

// orderedDict preserves insertion order. Used by decoders so `Profile` sees
// keys in the order they appeared in the source plist — the spec requires
// round-trip semantic equivalence including order.
type orderedDict struct {
	entries []orderedEntry
	index   map[string]int
}

type orderedEntry struct {
	Key   string
	Value any
}

func (d *orderedDict) set(key string, value any) {
	if d.index == nil {
		d.index = make(map[string]int)
	}
	if i, ok := d.index[key]; ok {
		d.entries[i].Value = value
		return
	}
	d.entries = append(d.entries, orderedEntry{Key: key, Value: value})
	d.index[key] = len(d.entries) - 1
}

func (d *orderedDict) get(key string) (any, bool) {
	i, ok := d.index[key]
	if !ok {
		return nil, false
	}
	return d.entries[i].Value, true
}

// DecodeMobileConfig parses an Apple Configuration Profile and returns the
// embedded com.anthropic.claudefordesktop payload as a Profile. Unknown keys
// are preserved on the Profile and reported in the DecodeReport.
func DecodeMobileConfig(data []byte) (*Profile, DecodeReport, error) {
	dict, err := parsePlistRootDict(data)
	if err != nil {
		return nil, DecodeReport{}, fmt.Errorf("decode mobileconfig: %w", err)
	}
	content, ok := dict.get("PayloadContent")
	if !ok {
		return nil, DecodeReport{}, fmt.Errorf("decode mobileconfig: no PayloadContent array")
	}
	arr, ok := content.([]any)
	if !ok {
		return nil, DecodeReport{}, fmt.Errorf("decode mobileconfig: PayloadContent is %T, want array", content)
	}

	var payload *orderedDict
	for _, item := range arr {
		od, ok := item.(*orderedDict)
		if !ok {
			continue
		}
		if t, _ := od.get("PayloadType"); t == "com.anthropic.claudefordesktop" {
			payload = od
			break
		}
	}
	if payload == nil {
		return nil, DecodeReport{}, fmt.Errorf("decode mobileconfig: no com.anthropic.claudefordesktop payload")
	}

	name, _ := firstString(payload, "PayloadDisplayName")
	if name == "" {
		name, _ = firstString(dict, "PayloadDisplayName")
	}
	p := New(name)
	report := DecodeReport{}
	for _, e := range payload.entries {
		if isPayloadMetadataKey(e.Key) {
			continue
		}
		assignDecodedValue(p, &report, e.Key, e.Value)
	}
	return p, report, nil
}

// DecodePlist parses the bare com.anthropic.claudefordesktop.plist body.
func DecodePlist(data []byte) (*Profile, DecodeReport, error) {
	dict, err := parsePlistRootDict(data)
	if err != nil {
		return nil, DecodeReport{}, fmt.Errorf("decode plist: %w", err)
	}
	p := New("")
	report := DecodeReport{}
	for _, e := range dict.entries {
		assignDecodedValue(p, &report, e.Key, e.Value)
	}
	return p, report, nil
}

// firstString is a small type-asserting helper used by decoders.
func firstString(d *orderedDict, key string) (string, bool) {
	v, ok := d.get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// DecodeReg is not yet implemented in v0.1; reserved for task-06.
func DecodeReg(_ []byte) (*Profile, DecodeReport, error) {
	return nil, DecodeReport{}, fmt.Errorf("decode .reg: not implemented (v0.3)")
}

// assignDecodedValue maps a raw plist-parsed value onto the profile,
// converting stringArray / jsonString types from their JSON-in-string
// encoding back to typed Go values. Unknown keys and type-mismatches are
// recorded on the report; a schema type mismatch does NOT fail decoding
// (caller's Validate() surfaces it).
func assignDecodedValue(p *Profile, report *DecodeReport, key string, value any) {
	s := schema.Load()
	k := s.Find(key)
	if k == nil {
		p.AttachUnknownKey(key, fmt.Sprintf("%v", value))
		report.UnknownKeys = append(report.UnknownKeys, UnknownKey{Key: key, RawValue: fmt.Sprintf("%v", value)})
		// still carry the raw value in the profile for round-trip fidelity
		p.SetRaw(key, value)
		return
	}
	switch k.Type {
	case schema.TypeStringArray:
		// stored as JSON-in-string
		if s, ok := value.(string); ok {
			arr, err := parseJSONStringArray(s)
			if err != nil {
				report.Warnings = append(report.Warnings,
					fmt.Sprintf("%s: stringArray value did not parse as JSON array: %s", key, err))
				p.SetRaw(key, value)
				return
			}
			p.SetRaw(key, arr)
			return
		}
		p.SetRaw(key, value)
	case schema.TypeJSONString:
		// keep as string; callers decide whether to parse further
		if s, ok := value.(string); ok {
			p.SetRaw(key, s)
			return
		}
		p.SetRaw(key, value)
	default:
		p.SetRaw(key, value)
	}
}

func isPayloadMetadataKey(key string) bool {
	switch key {
	case "PayloadType", "PayloadIdentifier", "PayloadUUID",
		"PayloadDisplayName", "PayloadVersion", "PayloadOrganization",
		"PayloadDescription", "PayloadScope", "PayloadEnabled":
		return true
	}
	return false
}

// parseJSONStringArray parses a raw JSON array-of-strings stored in a plist
// <string> element. Encoders always produce well-formed JSON; this decoder
// requires the same — no fallback tolerance for malformed input.
func parseJSONStringArray(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("not an array literal")
	}
	var arr []string
	if err := jsonUnmarshal(s, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// parsePlistRootDict uses encoding/xml with a tolerant handler that emits a
// Go value tree mirroring the plist structure. It only supports the subset
// of plist we generate: dict / array / string / integer / true / false /
// real / data. Keys carried in <key>...</key> siblings of dict children.
// Returns an ordered dict so consumers (Profile) see keys in source order.
func parsePlistRootDict(data []byte) (*orderedDict, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false // allow XHTML-ish tolerances
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local != "plist" {
			continue
		}
		// first child should be the root <dict>
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if se2, ok := tok.(xml.StartElement); ok {
				if se2.Name.Local != "dict" {
					return nil, fmt.Errorf("expected root <dict>, got <%s>", se2.Name.Local)
				}
				return parseDict(dec)
			}
		}
	}
	return nil, fmt.Errorf("no <plist> root element found")
}

func parseDict(dec *xml.Decoder) (*orderedDict, error) {
	out := &orderedDict{}
	var pendingKey string
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "key":
				s, err := readCharData(dec)
				if err != nil {
					return nil, err
				}
				pendingKey = s
			default:
				val, err := parseValue(dec, t)
				if err != nil {
					return nil, err
				}
				if pendingKey == "" {
					return nil, fmt.Errorf("value <%s> with no preceding <key>", t.Name.Local)
				}
				out.set(pendingKey, val)
				pendingKey = ""
			}
		case xml.EndElement:
			if t.Name.Local == "dict" {
				return out, nil
			}
		}
	}
}

func parseValue(dec *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "string":
		return readCharData(dec)
	case "integer":
		s, err := readCharData(dec)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case "real":
		s, err := readCharData(dec)
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, err
		}
		return f, nil
	case "true":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return true, nil
	case "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return false, nil
	case "array":
		var arr []any
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch tt := tok.(type) {
			case xml.StartElement:
				v, err := parseValue(dec, tt)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			case xml.EndElement:
				if tt.Name.Local == "array" {
					return arr, nil
				}
			}
		}
	case "dict":
		return parseDict(dec)
	case "data":
		// preserve as string; we don't currently emit data values but decoders
		// should tolerate them
		return readCharData(dec)
	default:
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// readCharData consumes CharData until the matching end element and returns
// the concatenated text. Safe against empty elements (<string/>).
func readCharData(dec *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		}
	}
}

// jsonUnmarshal is declared as a variable so tests can swap in a faster
// alternative if necessary. Defaults to encoding/json.Unmarshal.
var jsonUnmarshal = func(s string, v any) error {
	return jsonUnmarshalImpl(s, v)
}
