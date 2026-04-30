# Spec: `internal/profile/`

## Intent

Represent a Claude Desktop MDM configuration as a typed, format-agnostic Go value. Encode to any supported format; decode from any supported format. Round-trip safe for all keys defined in `internal/schema/`.

## Public interface

```go
package profile

import "github.com/krislavten/cowork-mdm/internal/schema"

// Profile is an in-memory MDM configuration. Field order is preserved for deterministic output.
type Profile struct {
    // Name is used as the PayloadDisplayName in mobileconfig and as a filename hint.
    Name string
    // Values holds key → typed value. Use Set/Get rather than direct access.
    // Stored as JSON-compatible types: string, bool, int64, []string, json.RawMessage.
    values []Entry
}

// Entry preserves insertion order so encoded output is deterministic.
type Entry struct {
    Key   string
    Value any
}

// New returns an empty profile with the given display name.
func New(name string) *Profile

// Set stores value under key. Validates against schema. Replaces existing.
// Returns error if key is unknown or value doesn't match the key's schema type.
func (p *Profile) Set(key string, value any) error

// Get returns the value stored under key and whether it exists.
func (p *Profile) Get(key string) (any, bool)

// Keys returns keys in insertion order.
func (p *Profile) Keys() []string

// Delete removes a key. No-op if absent.
func (p *Profile) Delete(key string)

// Validate re-validates all entries against the current schema.
// Useful after deserialization or when schema updates.
func (p *Profile) Validate() error
```

## Encoders

Each encoder is a single function in its own file:

```go
// encode_mobileconfig.go
func EncodeMobileConfig(p *Profile, opts MobileConfigOpts) ([]byte, error)

type MobileConfigOpts struct {
    PayloadIdentifier    string // default "com.yuanli.cowork-mdm.<slug>"
    PayloadUUID          string // default random v4
    PayloadOrganization  string // default empty
    PayloadScope         string // "System" (default) | "User"
}

// encode_plist.go — raw com.anthropic.claudefordesktop.plist (no MobileConfig wrapper)
func EncodePlist(p *Profile) ([]byte, error)

// encode_jamf.go — Jamf Custom Settings Payload format (plist body paste-able into Jamf UI)
func EncodeJamf(p *Profile) ([]byte, error)

// encode_intune.go — Intune "Preference File" XML for macOS (domain com.anthropic.claudefordesktop).
// Produces the exact XML that Intune expects when uploading a plist-based config for macOS devices.
// Windows Intune support is out of scope for v0.2.
func EncodeIntune(p *Profile) ([]byte, error)

// encode_reg.go — Windows .reg file targeting HKLM:\SOFTWARE\Policies\Claude
func EncodeReg(p *Profile, opts RegOpts) ([]byte, error)

type RegOpts struct {
    Hive string // "HKLM" (default) | "HKCU"
}
```

## Decoders

Each decoder attempts to construct a `*Profile` from a byte slice. Format is auto-detected by extension in the CLI layer, but decoders are individually callable.

```go
// decode.go
//
// Decoders return the decoded Profile, a DecodeReport (unknown keys + warnings),
// and an error. Unknown keys are NOT dropped — they are preserved on the Profile
// via p.UnknownKeys() so downstream commands (profile validate / profile status)
// can surface them. See the "Unknown key preservation" section below.
func DecodeMobileConfig(data []byte) (*Profile, DecodeReport, error)
func DecodePlist(data []byte) (*Profile, DecodeReport, error)
func DecodeReg(data []byte) (*Profile, DecodeReport, error)

// Detect inspects data and returns the likely format.
// Returns "mobileconfig", "plist", "reg", or "" (unknown).
func Detect(data []byte) string
```

## Encoding rules

### macOS plist / mobileconfig

Type mapping:

| Schema type | plist XML |
|---|---|
| `string` | `<string>...</string>` |
| `boolean` | `<true/>` or `<false/>` |
| `integer` | `<integer>N</integer>` |
| `stringArray` | **JSON-serialized as a string**: `<string>["a","b"]</string>` (not `<array>`) |
| `jsonString` | `<string>{...}</string>` (raw JSON in a string element) |
| `url` | `<string>https://...</string>` |
| `enum` | `<string>bedrock</string>` |

**Critical**: arrays go as JSON strings, not `<array>` elements. This is how Claude.app's `jee(...)` / `JSON.parse` reader expects them. A native plist array will fail `JSON.parse` on the Anthropic side.

For `.mobileconfig`, wrap the per-key dict in the standard Apple payload structure:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "...">
<plist version="1.0">
<dict>
    <key>PayloadContent</key>
    <array>
        <dict>
            <key>PayloadType</key><string>com.anthropic.claudefordesktop</string>
            <key>PayloadIdentifier</key><string>{opts.PayloadIdentifier}.settings</string>
            <key>PayloadUUID</key><string>{opts.PayloadUUID}</string>
            <key>PayloadDisplayName</key><string>{p.Name}</string>
            <key>PayloadVersion</key><integer>1</integer>
            <!-- per-key entries here -->
        </dict>
    </array>
    <key>PayloadDisplayName</key><string>{p.Name}</string>
    <key>PayloadIdentifier</key><string>{opts.PayloadIdentifier}</string>
    <key>PayloadType</key><string>Configuration</string>
    <key>PayloadUUID</key><string>{random}</string>
    <key>PayloadVersion</key><integer>1</integer>
    <key>PayloadScope</key><string>{opts.PayloadScope}</string>
</dict>
</plist>
```

Match the exact structure Claude.app's `o4n()` function produces (we reverse-engineered this in earlier exploration).

### Windows .reg

Header: `Windows Registry Editor Version 5.00\r\n\r\n`

Key line is driven by `RegOpts.Hive`:
- `Hive=""` (default) or `"HKLM"` → `[HKEY_LOCAL_MACHINE\SOFTWARE\Policies\Claude]\r\n`
- `Hive="HKCU"` → `[HKEY_CURRENT_USER\SOFTWARE\Policies\Claude]\r\n`

Other hives are rejected with an error from `EncodeReg`.

Value lines:

| Schema type | .reg representation |
|---|---|
| `string` / `url` / `enum` | `"name"="value"` |
| `boolean` | `"name"=dword:00000001` / `00000000` |
| `integer` | `"name"=dword:XXXXXXXX` (hex 8-digit) |
| `stringArray` | `"name"="[\"a\",\"b\"]"` (JSON string, escaped quotes) |
| `jsonString` | `"name"="{...}"` (JSON string, escaped quotes) |

Line endings are CRLF per Windows convention. Values with embedded quotes/backslashes escape as `\"` / `\\`.

### Jamf Custom Settings

Exact same format as the **inner** dict of a mobileconfig payload (no `<?xml` wrapper, no Configuration container). The Jamf UI "Custom Settings" payload accepts a plist body keyed directly by MDM preference name → value. Our encoder produces:

```xml
<dict>
    <key>inferenceProvider</key>
    <string>bedrock</string>
    <!-- ... -->
</dict>
```

Jamf wraps this on upload.

### Intune Preference File (macOS)

Intune for macOS ships MDM plist config as a "Preference File" upload. The XML uploaded to Intune has a `<managedPreferences>` wrapper around the key/value dict. cowork-mdm emits exactly that wrapper:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<managedPreferences>
    <forced>
        <mcx_preference_settings>
            <dict>
                <key>inferenceProvider</key><string>bedrock</string>
                <!-- ... -->
            </dict>
        </mcx_preference_settings>
    </forced>
</managedPreferences>
```

The inner dict uses the same per-key type rules as the `EncodePlist` output. Windows Intune is out of scope for v0.2 — users targeting Windows Intune should use `EncodeReg` and import the `.reg` via Intune's "Deploy a custom registry setting" device configuration.

## Validation

`Profile.Validate()` walks entries and calls `Key.Validate(value)` for each. Aggregates errors via `errors.Join`. Unknown keys (not in schema) are reported as errors of type `*UnknownKeyError` so callers can distinguish them from type-mismatch errors.

```go
type UnknownKeyError struct {
    Key string
}
func (e *UnknownKeyError) Error() string { return fmt.Sprintf("unknown key: %s", e.Key) }
```

## Unknown key preservation in decoders

Decoders do **not** silently drop unknown keys. They attach them to the returned `*Profile` via an internal side channel so `Profile.UnknownKeys()` exposes them. `Profile.Validate()` reports them via `UnknownKeyError`. The `profile status` and `profile validate` CLI commands rely on this to surface drift from the schema.

```go
// UnknownKey describes a key found in an input file that isn't in the schema.
type UnknownKey struct {
    Key      string // key name as seen in input
    RawValue string // stringified value, for reporting only
}

// UnknownKeys returns keys decoded from input but not present in the schema.
func (p *Profile) UnknownKeys() []UnknownKey

// DecodeReport is returned alongside *Profile from decoders.
type DecodeReport struct {
    UnknownKeys  []UnknownKey
    Warnings     []string // e.g. "PayloadScope=User is unusual for this PayloadType"
}

// Decoders now return both the Profile and a report:
func DecodeMobileConfig(data []byte) (*Profile, DecodeReport, error)
func DecodePlist(data []byte) (*Profile, DecodeReport, error)
func DecodeReg(data []byte) (*Profile, DecodeReport, error)
```

## Template loading

```go
// templates.go
//go:embed templates/*.yaml
var templateFS embed.FS

// LoadTemplate instantiates a template by name. Name matches file basename without ".yaml".
func LoadTemplate(name string) (*Profile, error)

// TemplateNames returns all available template names.
func TemplateNames() []string
```

Template YAML format:

```yaml
name: bedrock-basic
description: "Bedrock inference + basic MCP, no egress lock"
values:
  inferenceProvider: bedrock
  disableDeploymentModeChooser: true
  inferenceBedrockRegion: us-west-2
  inferenceBedrockProfile: default
  inferenceModels: '["arn:aws:bedrock:us-west-2:ACCOUNT:application-inference-profile/OPUS"]'
```

Templates ship with placeholder ACCOUNT values. CLI's `profile new` prompts (TUI or flag) to substitute.

## Testing

- `encode_mobileconfig_test.go`: compare output to `testdata/bedrock-basic.golden.mobileconfig`. `plutil -lint` subprocess check on the output (macOS CI only).
- Roundtrip: `Encode → Decode → Encode` produces **semantically equivalent** output (same MDM key/value pairs; PayloadUUID / PayloadIdentifier may differ because UUIDs are regenerated and must not be stable across encodes).
- Windows `.reg` output parsed by a fixture parser (pure Go) — verify line endings CRLF + dword hex width.
- Validation edge cases from schema.md's spec.

## Non-goals

- No mobileconfig signing support in v0.2.
- No multi-payload mobileconfig (one `PayloadContent` entry for Claude only).
