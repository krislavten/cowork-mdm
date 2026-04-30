// Package schema provides a typed, embedded representation of Claude
// Desktop's MDM key schema. Consumers (profile encoder, doctor,
// validator) read from here; they never touch the Claude.app binary
// directly. The authoritative data file (schema.json) is extracted from
// a Claude Desktop release via internal/schema/extract and embedded at
// build time.
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sync"
)

// Type identifies the MDM value type. Values map to the expression the
// extractor saw for each key in Claude Desktop's source.
type Type string

const (
	TypeString      Type = "string"      // MA() — plain string.
	TypeBoolean     Type = "boolean"     // Hi() — boolean.
	TypeInteger     Type = "integer"     // PsA.number().int() — 64-bit integer.
	TypeStringArray Type = "stringArray" // Li(MA()) — JSON array of strings.
	TypeJSONString  Type = "jsonString"  // Serialized JSON (managedMcpServers, inferenceModels).
	TypeURL         Type = "url"         // YsA / r_ — HTTPS URL string.
	TypeEnum        Type = "enum"        // Ds([...]) — restricted string values.
)

// Scope identifies which deployment mode a key applies to.
type Scope string

const (
	ScopeThirdParty          Scope = "3p"
	ScopeFirstParty          Scope = "1p"
	ScopeThirdPartyBootstrap Scope = "3p-bootstrap"
)

// Key describes one MDM configuration key.
type Key struct {
	Name             string   `json:"name"`
	Type             Type     `json:"type"`
	Scopes           []Scope  `json:"scopes"`
	AppMin           string   `json:"appMin,omitempty"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Example          any      `json:"example,omitempty"`
	LegacyAlias      string   `json:"legacyAlias,omitempty"`
	Sensitive        bool     `json:"sensitive,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Category         string   `json:"category,omitempty"`
	EnumValues       []string `json:"enumValues,omitempty"`
	Default          any      `json:"default,omitempty"`
	ExtractedFromApp string   `json:"-"`
}

// Schema is the loaded top-level structure.
type Schema struct {
	Version                 string `json:"version"`
	ExtractedFromAppVersion string `json:"extractedFromAppVersion"`
	Keys                    []Key  `json:"keys"`
}

//go:embed schema.json
var embeddedJSON []byte

var (
	loadOnce   sync.Once
	loaded     *Schema
	loadErr    error
	indexByKey map[string]*Key
)

// Load returns the embedded schema. The returned value is shared
// between callers — do not mutate it. Panics if the embedded JSON is
// malformed (that indicates a broken binary, which is unrecoverable).
func Load() *Schema {
	loadOnce.Do(parseEmbedded)
	if loadErr != nil {
		panic(fmt.Errorf("schema: embedded schema.json is invalid: %w", loadErr))
	}
	return loaded
}

func parseEmbedded() {
	var s Schema
	if err := json.Unmarshal(embeddedJSON, &s); err != nil {
		loadErr = err
		return
	}
	// Propagate the schema-level app version to each key so downstream
	// consumers (e.g. doctor reporting) can answer "which Claude
	// release did we extract this from?" without carrying the parent
	// Schema around.
	for i := range s.Keys {
		s.Keys[i].ExtractedFromApp = s.ExtractedFromAppVersion
	}

	idx := make(map[string]*Key, len(s.Keys))
	for i := range s.Keys {
		idx[s.Keys[i].Name] = &s.Keys[i]
	}

	loaded = &s
	indexByKey = idx
}

// Find returns the Key with the given name, or nil if absent. The
// returned pointer references the shared Schema — do not mutate it.
func (s *Schema) Find(name string) *Key {
	if s == nil {
		return nil
	}
	// Fast path: if s is the package's cached Schema, reuse the
	// pre-built index so lookups are O(1).
	if s == loaded && indexByKey != nil {
		return indexByKey[name]
	}
	for i := range s.Keys {
		if s.Keys[i].Name == name {
			return &s.Keys[i]
		}
	}
	return nil
}

// Validate checks a value against a Key's type and enum constraints.
// Returns nil on success or a descriptive error. Validate does not
// parse/inspect jsonString or url content — v0.2 only checks that the
// Go type is a string for those; full JSON/URL validation is left to
// the caller (profile package) when needed.
func (k *Key) Validate(value any) error {
	if k == nil {
		return fmt.Errorf("schema: Validate called on nil Key")
	}
	if value == nil {
		return fmt.Errorf("schema: key %q: value is nil", k.Name)
	}

	switch k.Type {
	case TypeBoolean:
		if _, ok := value.(bool); !ok {
			return typeError(k, "bool", value)
		}
		return nil

	case TypeInteger:
		return validateInteger(k, value)

	case TypeString:
		if _, ok := value.(string); !ok {
			return typeError(k, "string", value)
		}
		return nil

	case TypeStringArray:
		return validateStringArray(k, value)

	case TypeJSONString:
		// v0.2: we only require a string here. Encoders that need to
		// emit a plist <array>/<dict> parse the JSON themselves.
		if _, ok := value.(string); !ok {
			return typeError(k, "string (serialized JSON)", value)
		}
		return nil

	case TypeURL:
		// v0.2: we only require a string. Higher layers may later
		// enforce "https scheme required" — out of scope here.
		if _, ok := value.(string); !ok {
			return typeError(k, "string (URL)", value)
		}
		return nil

	case TypeEnum:
		s, ok := value.(string)
		if !ok {
			return typeError(k, "string (enum)", value)
		}
		for _, allowed := range k.EnumValues {
			if s == allowed {
				return nil
			}
		}
		return fmt.Errorf(
			"schema: key %q: value %q is not one of allowed enum values %v",
			k.Name, s, k.EnumValues,
		)

	default:
		return fmt.Errorf("schema: key %q has unknown type %q", k.Name, k.Type)
	}
}

// validateInteger accepts the integer-like Go numeric kinds. Floats
// (even whole-valued like 1.0) are rejected because JSON-decoded
// numbers land in float64 and callers should convert explicitly before
// passing them in.
func validateInteger(k *Key, value any) error {
	switch v := value.(type) {
	case int, int8, int16, int32, int64:
		return nil
	case uint, uint8, uint16, uint32:
		return nil
	case uint64:
		if v > math.MaxInt64 {
			return fmt.Errorf("schema: key %q: uint64 value %d exceeds int64 range", k.Name, v)
		}
		return nil
	default:
		return typeError(k, "integer", value)
	}
}

// validateStringArray accepts only []string. []any with all-string
// elements is intentionally rejected: callers should pass the concrete
// type so misuse (e.g. passing an array of numbers) is caught.
func validateStringArray(k *Key, value any) error {
	if _, ok := value.([]string); ok {
		return nil
	}
	return typeError(k, "[]string", value)
}

func typeError(k *Key, want string, got any) error {
	return fmt.Errorf("schema: key %q: expected %s, got %T", k.Name, want, got)
}
