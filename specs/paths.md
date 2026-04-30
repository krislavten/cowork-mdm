# Spec: `internal/paths/`

## Intent

Centralize all platform-specific filesystem/registry paths that other packages need. Prevents ad-hoc `runtime.GOOS == "darwin"` checks scattered across the codebase.

## Public interface

```go
package paths

// Provider returns platform-resolved paths. Returned paths are absolute.
type Provider interface {
    // ManagedPrefsPlist returns the path for Claude Desktop's managed plist on macOS.
    // Returns empty string on non-macOS platforms.
    ManagedPrefsPlist() string

    // ManagedPrefsUserPlist returns the per-user variant:
    // /Library/Managed Preferences/<username>/com.anthropic.claudefordesktop.plist
    // The app reads both; user-specific overrides system.
    ManagedPrefsUserPlist(username string) string

    // OrgPluginsDir returns Claude.app's hardcoded plugin directory.
    //   darwin:  /Library/Application Support/Claude/org-plugins
    //   windows: C:\Program Files\Claude\org-plugins
    //   linux:   empty (unsupported)
    OrgPluginsDir() string

    // ClaudeAppPath returns the expected Claude Desktop install location.
    //   darwin:  /Applications/Claude.app
    //   windows: C:\Program Files\Claude\Claude.exe
    ClaudeAppPath() string

    // UserSessionsDir returns the Claude-3p sessions dir for the current user.
    //   darwin:  $HOME/Library/Application Support/Claude-3p/local-agent-mode-sessions
    //   windows: $USERPROFILE\AppData\Roaming\Claude-3p\local-agent-mode-sessions
    UserSessionsDir() string

    // WindowsRegistryPath returns the MDM registry key path on Windows.
    //   windows: SOFTWARE\Policies\Claude
    //   other:   empty
    WindowsRegistryPath() string

    // LaunchAgentDir returns where to install per-user LaunchAgents (macOS).
    //   darwin:  /Library/LaunchAgents
    //   other:   empty
    LaunchAgentDir() string
}

// Default returns the Provider for the current operating system.
func Default() Provider

// ForOS returns the Provider for a specific OS. Used in tests for cross-platform simulation.
//   os must be one of "darwin", "windows", "linux".
func ForOS(os string) Provider
```

## Implementation

Split across three files using Go build constraints:

```go
// paths_darwin.go
//go:build darwin

// paths_windows.go
//go:build windows

// paths_other.go
//go:build !darwin && !windows
```

`ForOS` lives in `paths.go` and returns a concrete struct per OS. Each OS's struct is defined in `paths.go` (not build-gated) so all platforms can simulate all OSes in tests. `Default()` is the only build-gated entry; it returns the current-OS provider.

## Current-user resolution

- macOS: use `os.Getenv("USER")` or `user.Current().Username`. Fail soft: if neither available, return the system-scoped plist path only.
- Windows: use `os.Getenv("USERPROFILE")` and `os.Getenv("USERNAME")`.

## Testing

- Table-driven tests per OS via `ForOS("darwin")` / `ForOS("windows")`, asserting expected literal strings. Pin paths so downstream code has deterministic references.
- No filesystem access in tests.

## Non-goals

- Does not create directories. Package is pure path arithmetic.
- Does not read the filesystem to resolve anything. Returns paths even if they don't exist.
