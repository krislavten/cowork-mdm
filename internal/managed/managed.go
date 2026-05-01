// Package managed handles the side-effectful operations for
// `profile apply` and `profile status`: writing to the host's managed
// preferences store (macOS plist or Windows registry) and reading it back.
//
// CLI layer should only orchestrate this package — it must not perform
// filesystem or registry writes itself. This keeps the command-line code
// pure argument-parsing + output-formatting, as specified in specs/cli.md.
package managed

import (
	"errors"

	"github.com/krislavten/cowork-mdm/internal/profile"
)

// ApplyOptions controls how a profile is committed to the host's
// managed-prefs store.
type ApplyOptions struct {
	// DryRun emits what would happen without touching disk/registry.
	DryRun bool

	// TargetPath overrides the default managed-prefs path. Used in tests.
	// On darwin it's the path to a plist file; on windows the registry
	// subkey under the chosen hive (e.g. "SOFTWARE\\Policies\\Claude").
	// Empty = platform default.
	TargetPath string

	// Hive selects the Windows registry hive: "HKLM" (default) or "HKCU".
	// Ignored on non-Windows.
	Hive string
}

// StatusOptions mirrors ApplyOptions for the read side. SourcePath and Hive
// accept the same values.
type StatusOptions struct {
	SourcePath string
	Hive       string
}

// ApplyResult summarizes what was done.
type ApplyResult struct {
	// TargetPath is the absolute path or registry key that was (or would
	// have been) written.
	TargetPath string

	// BytesWritten is the size of the emitted plist on darwin, 0 on
	// windows (registry writes aren't byte-measured).
	BytesWritten int

	// Platform is "darwin" or "windows".
	Platform string

	// DryRun mirrors the input option.
	DryRun bool

	// Preview holds the rendered plist (darwin) or .reg (windows) text
	// that would have been written. Populated only when DryRun is true.
	Preview string

	// Warnings lists non-fatal caveats the caller should surface. For
	// example, direct writes to /Library/Managed Preferences/ bypass
	// macOS's managedappconfigd and may be clobbered on profile resync;
	// production deployments should push a .mobileconfig via an MDM
	// channel instead.
	Warnings []string
}

// StatusReport describes what's currently active on the host.
type StatusReport struct {
	Platform    string
	TargetPath  string
	Present     bool
	Profile     *profile.Profile
	UnknownKeys []profile.UnknownKey
	ParseError  string
}

// Sentinel errors — callers can errors.Is() to distinguish.
var (
	// ErrPermission is returned when the operation requires elevation
	// (root on macOS; admin on Windows for HKLM writes).
	ErrPermission = errors.New("managed: operation requires elevated privileges")

	// ErrUnsupportedPlatform is returned on Linux or other OSes where
	// neither the plist path nor the registry key exists.
	ErrUnsupportedPlatform = errors.New("managed: apply/status not supported on this platform")
)

// Apply writes the profile to the host in the platform-appropriate form.
//   - On darwin, encodes as raw plist (EncodePlist) and writes to the
//     managed-prefs path (/Library/Managed Preferences/com.anthropic.claudefordesktop.plist).
//   - On windows, writes each profile entry to the chosen registry hive.
//   - On linux, returns ErrUnsupportedPlatform.
//
// Requires elevated privileges. Returns ErrPermission (wrapped) if not elevated.
func Apply(p *profile.Profile, opts ApplyOptions) (*ApplyResult, error) {
	return apply(p, opts)
}

// Status reads the current managed preferences on the host.
// Pass a zero-value StatusOptions for defaults.
func Status(opts StatusOptions) (*StatusReport, error) {
	return status(opts)
}
