# Spec: `internal/schema/`

## Intent

Provide a typed, embedded representation of Claude Desktop's MDM key schema. Consumers (profile encoder, doctor, validator) read this — they never touch the Claude.app binary directly.

## Public interface

```go
package schema

// Key describes one MDM configuration key.
type Key struct {
    Name                string   `json:"name"`
    Type                Type     `json:"type"`                // "string" | "boolean" | "integer" | "stringArray" | "jsonString" | "url" | "enum"
    Scopes              []Scope  `json:"scopes"`              // ["3p"], ["1p"], ["3p","1p"], ["3p-bootstrap"]
    AppMin              string   `json:"appMin,omitempty"`    // semver, e.g. "1.2.0"
    Title               string   `json:"title"`
    Description         string   `json:"description"`
    Example             any      `json:"example,omitempty"`   // string OR []string
    LegacyAlias         string   `json:"legacyAlias,omitempty"`
    Sensitive           bool     `json:"sensitive,omitempty"`
    Provider            string   `json:"provider,omitempty"`  // "gateway" | "vertex" | "bedrock" | "foundry"
    Category            string   `json:"category,omitempty"`
    EnumValues          []string `json:"enumValues,omitempty"`// for Ds(...) types
    ExtractedFromApp    string   `json:"-"`                   // populated from parent JSON
}

type Type string
const (
    TypeString      Type = "string"       // MA() — plain string
    TypeBoolean     Type = "boolean"      // Hi() — boolean
    TypeInteger     Type = "integer"      // PsA.number().int()
    TypeStringArray Type = "stringArray"  // Li(MA()) — JSON array of strings
    TypeJSONString  Type = "jsonString"   // serialized JSON (managedMcpServers, inferenceModels)
    TypeURL         Type = "url"          // YsA / r_ — HTTPS URL string
    TypeEnum        Type = "enum"         // Ds([...]) — restricted string values
)

type Scope string
const (
    ScopeThirdParty          Scope = "3p"
    ScopeFirstParty          Scope = "1p"
    ScopeThirdPartyBootstrap Scope = "3p-bootstrap"
)

// Schema is the loaded top-level structure.
type Schema struct {
    Version                 string   `json:"version"`                 // schema format version, "1"
    ExtractedFromAppVersion string   `json:"extractedFromAppVersion"` // "1.5354.0"
    Keys                    []Key    `json:"keys"`
}

// Load returns the embedded schema. The returned value is shared — do not mutate.
func Load() *Schema

// Find returns the Key with the given name, or nil if absent.
func (s *Schema) Find(name string) *Key

// Validate checks a value against a Key's type + enum constraints.
// Returns nil on success or a descriptive error.
func (k *Key) Validate(value any) error
```

## schema.json format

```json
{
  "version": "1",
  "extractedFromAppVersion": "1.5354.0",
  "keys": [
    {
      "name": "inferenceProvider",
      "type": "enum",
      "scopes": ["3p", "3p-bootstrap"],
      "appMin": "1.2.0",
      "title": "Inference provider",
      "description": "Selects the inference backend. Setting this key activates third-party mode.",
      "enumValues": ["bedrock", "vertex", "gateway", "foundry"]
    },
    {
      "name": "managedMcpServers",
      "type": "jsonString",
      "scopes": ["3p"],
      "appMin": "1.2.0",
      "title": "Managed MCP servers",
      "description": "JSON array of MCP server configs...",
      "sensitive": true,
      "example": "[{\"name\":\"internal-tools\",\"url\":\"https://...\",\"transport\":\"sse\"}]"
    }
  ]
}
```

## Embedding + extraction

- `schema.json` is embedded into the binary via `//go:embed schema.json` in `schema.go`.
- `Load()` parses the embed on first call and caches (use `sync.Once`).
- Extraction is a separate tool: `internal/schema/extract/`. Build tag `//go:build extract` keeps it out of the main binary. Maintainer runs `go run -tags extract ./internal/schema/extract --from /Applications/Claude.app` to regenerate `schema.json`.

## Extraction algorithm (for the maintainer tool)

1. Shell out to `npx @electron/asar extract <app.asar> <tmpdir>` OR pure-Go asar unpack.
2. Read `<tmpdir>/.vite/build/index.js`.
3. Find substring `FJ=me({` (or `FJ = me({` after prettification). Use `strings.Index`.
4. Brace-match from `(` to find the end of the `me(...)` call. Use a bracket-counting loop (depth starts at 1 after the opening `(`).
5. Parse the captured block with regex for top-level `<keyName>:nn(<typeExpr>,{...})` entries. The type expr is one of:
   - `Hi()` → boolean
   - `MA()` → string (possibly chained `.trim().min(1)` etc.)
   - `YsA` or `r_` → URL string
   - `PsA.number().int()...` → integer
   - `Li(MA()...)` → string array
   - `Ds([...])` → enum (extract the literal array)
   - `ASt` → jsonString (managedMcpServers specifically — treat as JSON string)
   - `Vee` → jsonString (bootstrapOidc)
6. For each metadata block `{scopes: [...], title: ..., description: ..., ...}`, extract fields via regex (description often spans multiple lines with ``` ` ``` strings — handle backtick template literals).
7. Emit the JSON.

The extractor is fragile by design — it's a one-time-per-Claude-release maintenance task, not runtime logic.

## Error handling

- `Load()` panics on embedded JSON parse failure. This is unrecoverable — binary is broken.
- `Find(name)` returns nil for unknown keys. Callers decide what that means (unknown key in a profile = validation error).
- `Validate(value)` returns `fmt.Errorf` describing the violation. Wrap with profile context at call site.

## Testing

- `schema_test.go`: golden test. `Load()` produces ≥ 51 keys, `Find("inferenceProvider").Type == TypeEnum`, `Find("managedMcpServers").Sensitive == true`, etc.
- `validate_test.go`: table-driven. `{Key, Value, WantErr}` for each Type. Edge: integer overflow, boolean from string, enum mismatch, URL malformed.
- No extractor test in this package — extractor has its own `extract/extract_test.go` with a fixture `app.asar`.

## Non-goals

- Schema does not know about MDM output formats. It's a data model only.
- Schema does not know about Profile. Validation takes a `any`, caller composes.
- No live fetching from Anthropic. Schema is static, shipped with binary.
