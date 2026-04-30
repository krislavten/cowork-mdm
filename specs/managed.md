# Spec: `internal/managed/`

## Intent

Handle the side-effectful operations for `profile apply` and `profile status`: write to `/Library/Managed Preferences/` on macOS, write to `HKLM:\SOFTWARE\Policies\Claude` on Windows, and read both back. Separates this concern from both the CLI layer (which only orchestrates) and the profile encoders (which are pure functions).

## Public interface

```go
package managed

import "github.com/krislavten/cowork-mdm/internal/profile"

// ApplyOptions controls how a profile is committed to the host's managed-prefs store.
type ApplyOptions struct {
    // DryRun: emit what would happen without touching disk/registry. Default false.
    DryRun bool
    // TargetPath overrides the default managed-prefs path. Used in tests.
    // If empty, use paths.ManagedPrefsPlist() (macOS) or the registry default (windows).
    TargetPath string
    // Hive for Windows registry: "HKLM" (default) or "HKCU".
    Hive string
}

// ApplyResult summarizes what was done.
type ApplyResult struct {
    TargetPath   string   // path or registry key written
    BytesWritten int      // 0 when Windows
    Platform     string   // "darwin" | "windows"
    DryRun       bool
    Preview      string   // when DryRun, the textual plist or .reg that would be written
    Warnings     []string // e.g. "Direct write to /Library/Managed Preferences/ bypasses MDM and
                          // may be clobbered by managedappconfigd on the next sync. For
                          // production use, deploy this profile via your MDM channel instead."
}

// Apply writes the profile to the host in the platform-appropriate form.
// - On darwin, encodes as raw plist and writes to /Library/Managed Preferences/com.anthropic.claudefordesktop.plist.
// - On windows, writes each profile entry to the configured registry hive.
//
// Requires elevated privileges on both platforms. Returns PermissionError (see Errors) if not elevated.
func Apply(p *profile.Profile, opts ApplyOptions) (*ApplyResult, error)

// StatusReport describes what's currently active on the host.
type StatusReport struct {
    Platform     string                    // "darwin" | "windows"
    TargetPath   string                    // path or registry key read from
    Present      bool                      // plist / key exists
    Profile      *profile.Profile          // parsed contents (nil if Present=false)
    UnknownKeys  []profile.UnknownKey      // keys present on host but not in current schema
    ParseError   string                    // non-empty if Present but unparseable
}

// StatusOptions mirrors ApplyOptions for symmetry and testability.
type StatusOptions struct {
    // SourcePath overrides the default path (macOS) or registry key (Windows).
    // If empty, use the platform default.
    SourcePath string
    // Hive is used on Windows only: "HKLM" (default) or "HKCU".
    Hive string
}

// Status reads current managed preferences on the host.
// Pass a zero-value StatusOptions for default behavior.
func Status(opts StatusOptions) (*StatusReport, error)

// Errors
var (
    // ErrPermission is returned when the operation requires elevation.
    // Check via errors.Is(err, ErrPermission).
    ErrPermission = errors.New("managed: operation requires elevated privileges")
    // ErrUnsupportedPlatform when called on linux.
    ErrUnsupportedPlatform = errors.New("managed: apply/status not supported on this platform")
)
```

## Implementation notes

### Platform split

`managed.go` holds the public API + cross-platform plumbing.

`apply_darwin.go` (build tag `//go:build darwin`):
- Encode via `profile.EncodePlist(p)`.
- Ensure `/Library/Managed Preferences/` exists (it normally does; `os.MkdirAll` is no-op if present).
- Atomic write: temp file in same dir + `os.Rename`.
- Requires root or `/Library/Managed Preferences/` writable — check with `os.Geteuid() == 0` and wrap failures as `ErrPermission`.
- **Warn loudly** when writing to this path outside an MDM channel: include a note in `ApplyResult` that this bypasses macOS's `managedappconfigd` and may be clobbered on profile resync. Production deployments should use a `.mobileconfig` profile pushed via MDM.

`apply_windows.go` (build tag `//go:build windows`):
- Use `golang.org/x/sys/windows/registry`.
- For each profile entry, map type → registry value kind:
  - string / url / enum → `registry.SZ`
  - stringArray → `registry.SZ` holding JSON-encoded array (match Claude.app expectations)
  - jsonString → `registry.SZ` holding the JSON text
  - boolean → `registry.DWORD` with 0 or 1
  - integer → `registry.DWORD`
- HKLM requires admin (open with `registry.WRITE`); wrap `ERROR_ACCESS_DENIED` as `ErrPermission`.

`apply_other.go` (linux, etc.): returns `ErrUnsupportedPlatform`.

### DryRun

On darwin, Preview = rendered plist bytes.
On windows, Preview = rendered `.reg` text.

### Status

`status_darwin.go`:
- Read target plist. If absent, `Present=false`.
- If present, parse with `profile.DecodePlist`. Attach `UnknownKeys` from the DecodeReport.
- If parse fails, set `ParseError` and leave `Profile=nil`.

`status_windows.go`:
- Open `HKLM\SOFTWARE\Policies\Claude`. If absent, `Present=false`.
- Enumerate values, reverse-map to profile entries (invert the type mapping above).
- Same `UnknownKeys` / `ParseError` treatment.

## Testing

- `apply_test.go` + build-tagged platform files.
- `TestApplyDryRun`: in-memory, no side effects; check Preview content.
- `TestApplyWritesAtomic`: darwin test using a `TargetPath` override pointing at a temp file.
- `TestStatusReadsWritten`: write via Apply with overridden path, read via Status with same override.
- Windows: CI-only (use `Wine`? No — just rely on CI windows runner).

## Non-goals

- No `.mobileconfig` wrapper writing. That's done by producing the `.mobileconfig` file via `profile.EncodeMobileConfig` and letting the user push it through MDM. `Apply` targets the raw managed-prefs path only.
- No profile signing.
- No uninstall / revert. To remove, the user does `sudo rm` or registry delete.
