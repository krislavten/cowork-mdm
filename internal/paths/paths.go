// Package paths centralizes platform-specific filesystem and registry paths
// used throughout cowork-mdm. It exists so other packages can avoid scattered
// runtime.GOOS checks and so tests can simulate every supported OS from any
// host via ForOS.
//
// The package is pure path arithmetic: it never touches the filesystem and
// never creates directories. Returned paths are absolute on the OSes they
// apply to, and empty strings on OSes where a given path does not apply.
package paths

import (
	"os"
	"path"
	"strings"
)

// Provider returns platform-resolved paths. Returned paths are absolute on
// the target platform. For paths that do not apply to a given OS, the
// corresponding method returns an empty string.
type Provider interface {
	// ManagedPrefsPlist returns the path for Claude Desktop's managed plist on macOS.
	// Returns empty string on non-macOS platforms.
	ManagedPrefsPlist() string

	// ManagedPrefsUserPlist returns the per-user variant:
	//   /Library/Managed Preferences/<username>/com.anthropic.claudefordesktop.plist
	// The app reads both; user-specific overrides system.
	// Returns empty string on non-macOS platforms or when username is empty.
	ManagedPrefsUserPlist(username string) string

	// OrgPluginsDir returns Claude.app's hardcoded plugin directory.
	//   darwin:  /Library/Application Support/Claude/org-plugins
	//   windows: C:\Program Files\Claude\org-plugins
	//   linux:   empty (unsupported)
	OrgPluginsDir() string

	// ClaudeAppPath returns the expected Claude Desktop install location.
	//   darwin:  /Applications/Claude.app
	//   windows: C:\Program Files\Claude\Claude.exe
	//   linux:   empty (unsupported)
	ClaudeAppPath() string

	// UserSessionsDir returns the Claude-3p sessions dir for the current user.
	//   darwin:  $HOME/Library/Application Support/Claude-3p/local-agent-mode-sessions
	//   windows: $USERPROFILE\AppData\Roaming\Claude-3p\local-agent-mode-sessions
	//   linux:   empty (unsupported)
	// Returns empty string if the home directory cannot be resolved.
	UserSessionsDir() string

	// WindowsRegistryPath returns the MDM registry key path on Windows.
	//   windows: SOFTWARE\Policies\Claude
	//   other:   empty
	WindowsRegistryPath() string

	// LaunchAgentDir returns where to install per-user LaunchAgents on macOS.
	//   darwin:  /Library/LaunchAgents
	//   other:   empty
	LaunchAgentDir() string
}

// Literal path constants. Pinned here (not per-OS file) so tests can assert
// against them and so cross-OS simulation via ForOS works identically on
// every host.
const (
	darwinManagedPrefsPlist    = "/Library/Managed Preferences/com.anthropic.claudefordesktop.plist"
	darwinManagedPrefsUserRoot = "/Library/Managed Preferences"
	darwinManagedPrefsLeaf     = "com.anthropic.claudefordesktop.plist"
	darwinOrgPluginsDir        = "/Library/Application Support/Claude/org-plugins"
	darwinClaudeAppPath        = "/Applications/Claude.app"
	darwinLaunchAgentDir       = "/Library/LaunchAgents"
	darwinUserSessionsRelative = "Library/Application Support/Claude-3p/local-agent-mode-sessions"

	windowsOrgPluginsDir        = `C:\Program Files\Claude\org-plugins`
	windowsClaudeAppPath        = `C:\Program Files\Claude\Claude.exe`
	windowsRegistryPath         = `SOFTWARE\Policies\Claude`
	windowsUserSessionsRelative = `AppData\Roaming\Claude-3p\local-agent-mode-sessions`
)

// homeResolver returns the user home directory for the target OS's conventions.
// Exposed as a struct field so tests can inject a deterministic value.
type homeResolver func() (string, error)

// darwinProvider resolves macOS paths. Defined here (not build-gated) so
// ForOS can simulate it from any platform.
type darwinProvider struct {
	// home resolves $HOME. nil means "read os.Getenv(\"HOME\") only".
	home homeResolver
}

func (p darwinProvider) ManagedPrefsPlist() string {
	return darwinManagedPrefsPlist
}

func (p darwinProvider) ManagedPrefsUserPlist(username string) string {
	if username == "" {
		return ""
	}
	// Use path.Join (not filepath.Join) so the result is a POSIX path with
	// forward slashes regardless of the host OS running the simulation.
	return path.Join(darwinManagedPrefsUserRoot, username, darwinManagedPrefsLeaf)
}

func (p darwinProvider) OrgPluginsDir() string {
	return darwinOrgPluginsDir
}

func (p darwinProvider) ClaudeAppPath() string {
	return darwinClaudeAppPath
}

func (p darwinProvider) UserSessionsDir() string {
	home, err := p.resolveHome()
	if err != nil || home == "" {
		return ""
	}
	// path.Join keeps forward slashes so darwin simulation is deterministic
	// on any host (including Windows CI runners). We do NOT trim trailing
	// slashes from home because home="/" would then collapse to "" and
	// produce a relative path, violating the absolute-path guarantee.
	// path.Join already canonicalizes duplicate slashes internally.
	return path.Join(home, darwinUserSessionsRelative)
}

func (p darwinProvider) WindowsRegistryPath() string {
	return ""
}

func (p darwinProvider) LaunchAgentDir() string {
	return darwinLaunchAgentDir
}

func (p darwinProvider) resolveHome() (string, error) {
	if p.home != nil {
		return p.home()
	}
	// Only honour $HOME. Avoid os.UserHomeDir() host fallback so
	// ForOS("darwin") remains deterministic when executed from a
	// non-darwin host (which would otherwise inject a backslash path on
	// Windows). Callers get an empty UserSessionsDir() when $HOME is
	// unset, matching the spec's "returns empty if unresolved" rule.
	if h := os.Getenv("HOME"); h != "" {
		return h, nil
	}
	return "", nil
}

// windowsProvider resolves Windows paths. Defined here (not build-gated) so
// ForOS can simulate it from any platform.
type windowsProvider struct {
	// home resolves %USERPROFILE%. nil means "read os.Getenv(\"USERPROFILE\") only".
	home homeResolver
}

func (p windowsProvider) ManagedPrefsPlist() string {
	return ""
}

func (p windowsProvider) ManagedPrefsUserPlist(username string) string {
	return ""
}

func (p windowsProvider) OrgPluginsDir() string {
	return windowsOrgPluginsDir
}

func (p windowsProvider) ClaudeAppPath() string {
	return windowsClaudeAppPath
}

func (p windowsProvider) UserSessionsDir() string {
	home, err := p.resolveHome()
	if err != nil || home == "" {
		return ""
	}
	// Use backslash joining explicitly so the result is a valid Windows
	// path regardless of the host OS running the code (important for
	// ForOS("windows") on darwin/linux). Trim any trailing separator on
	// the home prefix so we never emit a double-backslash.
	home = strings.TrimRight(home, `\/`)
	return home + `\` + windowsUserSessionsRelative
}

func (p windowsProvider) WindowsRegistryPath() string {
	return windowsRegistryPath
}

func (p windowsProvider) LaunchAgentDir() string {
	return ""
}

func (p windowsProvider) resolveHome() (string, error) {
	if p.home != nil {
		return p.home()
	}
	// Only honour %USERPROFILE%. Avoid os.UserHomeDir() host fallback so
	// ForOS("windows") stays deterministic when running from a non-Windows
	// host (which would otherwise return a POSIX path). When unset,
	// UserSessionsDir() resolves to empty, matching the spec.
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h, nil
	}
	return "", nil
}

// otherProvider is the empty-path provider used on unsupported platforms
// (Linux, BSDs, etc.). Every method returns "" per spec.
type otherProvider struct{}

func (otherProvider) ManagedPrefsPlist() string                    { return "" }
func (otherProvider) ManagedPrefsUserPlist(username string) string { return "" }
func (otherProvider) OrgPluginsDir() string                        { return "" }
func (otherProvider) ClaudeAppPath() string                        { return "" }
func (otherProvider) UserSessionsDir() string                      { return "" }
func (otherProvider) WindowsRegistryPath() string                  { return "" }
func (otherProvider) LaunchAgentDir() string                       { return "" }

// ForOS returns the Provider for a specific operating system identifier.
// Intended for tests that need to simulate multiple OSes from a single host.
//
// Recognised values: "darwin", "windows". Any other value (including
// "linux") returns the empty-path otherProvider.
func ForOS(os string) Provider {
	switch os {
	case "darwin":
		return darwinProvider{}
	case "windows":
		return windowsProvider{}
	default:
		return otherProvider{}
	}
}
