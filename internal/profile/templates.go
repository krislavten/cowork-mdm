package profile

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// templateDoc is the on-disk YAML format. Values are stored as any so we can
// preserve JSON-shaped jsonString/stringArray values verbatim.
type templateDoc struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Values      map[string]any `yaml:"values"`
	ProfileName string         `yaml:"profile_name,omitempty"` // optional display name override
}

// LoadTemplate reads the named built-in template. Names match the file
// basename (without .yaml). An unknown name returns an error listing the
// available templates.
func LoadTemplate(name string) (*Profile, error) {
	data, err := readTemplate(name)
	if err != nil {
		return nil, err
	}
	return instantiate(data)
}

// LoadTemplateFile is the same but from a caller-supplied filesystem path.
// Used by `profile new --from path/to/my-profile.yaml` to keep enterprise-
// specific configs (with ARNs, tokens, etc.) outside the binary.
func LoadTemplateFile(rawYAML []byte) (*Profile, error) {
	var doc templateDoc
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	if doc.Name == "" {
		return nil, fmt.Errorf("template: missing required `name` field")
	}
	return instantiate(&doc)
}

// TemplateNames returns all built-in template names, sorted.
func TemplateNames() []string {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(n, ".yaml"))
	}
	sort.Strings(names)
	return names
}

func readTemplate(name string) (*templateDoc, error) {
	fname := path.Join("templates", name+".yaml")
	b, err := fs.ReadFile(templateFS, fname)
	if err != nil {
		avail := strings.Join(TemplateNames(), ", ")
		return nil, fmt.Errorf("template %q not found (available: %s)", name, avail)
	}
	var doc templateDoc
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("template %q: %w", name, err)
	}
	return &doc, nil
}

// instantiate turns a parsed templateDoc into a Profile, validating each
// value against the schema. Unknown keys error out immediately — templates
// must stay in sync with the schema.
func instantiate(doc *templateDoc) (*Profile, error) {
	profileName := doc.ProfileName
	if profileName == "" {
		profileName = doc.Name
	}
	p := New(profileName)
	s := schema.Load()

	// Preserve a stable order (YAML map iteration is unordered in yaml.v3).
	keys := make([]string, 0, len(doc.Values))
	for k := range doc.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := normalizeTemplateValue(doc.Values[k])
		// Templates are human-written YAML. For ergonomics we let authors
		// write JSON-shaped arrays (stringArray) or JSON objects
		// (jsonString) as a quoted string, and parse them here so schema
		// validation accepts them.
		if sk := s.Find(k); sk != nil {
			switch sk.Type {
			case schema.TypeStringArray:
				if str, ok := v.(string); ok {
					var arr []string
					if err := json.Unmarshal([]byte(str), &arr); err != nil {
						return nil, fmt.Errorf("template %q: %s: expected JSON array of strings, got %q: %w",
							doc.Name, k, str, err)
					}
					v = arr
				}
			case schema.TypeJSONString:
				// jsonString stays as a JSON-text string; if the YAML value
				// is already structured (map/slice), re-marshal to string.
				if _, ok := v.(string); !ok {
					b, err := json.Marshal(v)
					if err != nil {
						return nil, fmt.Errorf("template %q: %s: cannot marshal to JSON: %w",
							doc.Name, k, err)
					}
					v = string(b)
				}
			}
		}
		if err := p.Set(k, v); err != nil {
			return nil, fmt.Errorf("template %q: %w", doc.Name, err)
		}
	}
	return p, nil
}

// normalizeTemplateValue converts YAML-parsed numeric types (int in yaml.v3)
// so that schema integer validation accepts them. YAML bools and strings
// pass through. Nested maps/slices become []any / map[string]any and the
// caller (profile.Set → schema.Validate) handles them.
func normalizeTemplateValue(v any) any {
	switch t := v.(type) {
	case int:
		return int64(t)
	case uint:
		return int64(t)
	case float64:
		// YAML integers load as int, not float64, so any float64 here is
		// truly meant to be a float. But our schema has no float type, so
		// preserve it as-is and let validate complain.
		return t
	default:
		return v
	}
}
