# Spec: `internal/plugin/`

## Intent

List and manage individual plugin entries under `org-plugins/`. Reads marketplace state produced by `internal/marketplace/` and inspects per-user Claude-3p sessions to report actual enabled state.

Plugin management sits **on top of** marketplace management. `internal/marketplace/` owns clone + symlink lifecycle for repo-backed plugins. `internal/plugin/` adds per-plugin operations (show info, selectively unlink, prune dangling).

## Public interface

```go
package plugin

// Plugin describes one discoverable plugin entry.
type Plugin struct {
    Name         string // top-level name under org-plugins/
    Source       string // "marketplace:<repo>" | "local-directory" | "symlink-to-unknown"
    TargetPath   string // where the top-level entry points (if symlink) or the dir itself (if real)
    IsSymlink    bool
    Dangling     bool   // symlink target doesn't exist
    Manifest     *Manifest // parsed plugin.json, nil if unreadable
}

// Manifest mirrors the shape of .claude-plugin/plugin.json.
type Manifest struct {
    Name        string `json:"name"`
    Version     string `json:"version"`
    Description string `json:"description"`
    Author      struct {
        Name  string `json:"name"`
        Email string `json:"email"`
    } `json:"author"`
}

// EnabledState describes how a plugin is configured across all local Claude-3p sessions.
type EnabledState struct {
    Plugin       string
    BySession    []SessionEnabled
    AllEnabled   bool
    AnyDisabled  bool
}

type SessionEnabled struct {
    SessionPath string // absolute path to the session dir
    Enabled     *bool  // nil = not present in enabledPlugins; pointer to true/false = explicit
    Installed   bool   // true if entry exists in installed_plugins.json
}

// Inspector reads org-plugins/ without mutating.
type Inspector struct {
    orgPluginsDir   string
    sessionsDir     string // may be empty on unsupported platforms
}

func NewInspector(orgPluginsDir, sessionsDir string) *Inspector

// List returns all top-level entries under org-plugins/.
// Includes both symlinks (marketplace-linked plugins) and real dirs (marketplace clones or manually placed).
func (i *Inspector) List() ([]Plugin, error)

// Get returns one plugin by name or ErrPluginNotFound.
func (i *Inspector) Get(name string) (*Plugin, error)

// EnabledState enumerates per-user sessions and reports each plugin's state.
func (i *Inspector) EnabledStates() ([]EnabledState, error)

// Mutator allows destructive operations.
type Mutator struct {
    orgPluginsDir string
}

func NewMutator(orgPluginsDir string) *Mutator

// Unlink removes a top-level symlink. No-op on real directories (safer — don't delete user's own).
func (m *Mutator) Unlink(name string) error

// Prune removes all dangling top-level symlinks.
func (m *Mutator) Prune() (removed []string, err error)

var ErrPluginNotFound = errors.New("plugin not found")
```

## Discovery rules

`List()` logic:

1. `readdir(orgPluginsDir)`.
2. For each entry:
   - Skip hidden (name starts with `.`).
   - If symlink: `readlink()` → if target resolves to a dir containing `.claude-plugin/plugin.json`, read manifest. If target missing, mark `Dangling: true`.
   - If real directory: check if it has `.git` → treat as marketplace repo (report `Source: "local-directory"` with note — actually marketplace itself), else treat as locally-placed plugin. v0.2 surfaces both kinds of real directories as `Source: "local-directory"` and lets `marketplace.List()` separately enumerate marketplace repos.

`Source` string formats:
- `"marketplace:<repo-basename>"` — symlink target starts with `<orgPluginsDir>/<repo>/`
- `"local-directory"` — real directory at top level
- `"symlink-to-unknown"` — symlink target outside orgPluginsDir

## Enabled state enumeration

Scan `sessionsDir/*/*` — each pair `<accountUuid>/<orgUuid>` is a session.

Per session:
- Read `<session>/cowork_settings.json`. Parse `enabledPlugins` object.
- Read `<session>/cowork_plugins/installed_plugins.json`. Parse `plugins` object.

For each plugin in `List()`:
- `plugin_id = "<name>@org-provisioned"`
- `Enabled`: explicit true, explicit false, or nil (absent).
- `Installed`: key exists in installed_plugins.json.

`AllEnabled = all sessions have Enabled == true`
`AnyDisabled = any session has Enabled == false`

## Testing

- Fixture `testdata/org-plugins/` with a mix of:
  - `real-dir/` (no symlink)
  - `linked/` → `marketplace/plugins/linked`
  - `dangling/` → `marketplace/plugins/nonexistent` (target removed)
- `TestListEnumeratesAll`: expects 3 entries with correct Source strings.
- `TestPruneRemovesOnlyDangling`: verifies real dirs and valid symlinks are untouched.
- `TestEnabledStateMergesSessionsCorrectly`: fixture sessions dir with two sessions, one enabled, one disabled.

## Non-goals

- No install / uninstall in v0.2 (LaunchAgent `apply-defaults.py` from the pkg toolkit handles that out-of-band).
- No Windows support in v0.2.
