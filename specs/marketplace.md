# Spec: `internal/marketplace/`

## Intent

Manage Git-backed plugin marketplace repositories that sit under `org-plugins/`. Each marketplace repo follows Anthropic's "marketplace" convention: top-level `plugins/<name>/.claude-plugin/plugin.json` (and optionally `external_plugins/<name>/...`). cowork-mdm clones these repos into `org-plugins/<repo-basename>/`, then symlinks every discovered plugin into `org-plugins/<plugin-name>` so Claude Desktop sees them at the top level.

## Platform support

v0.2: **macOS only**. Windows requires junctions + admin + Developer Mode and is deferred.

## Public interface

```go
package marketplace

// Repo describes one cloned marketplace.
type Repo struct {
    Name       string    // directory basename, e.g. "claude-plugins-official"
    URL        string    // git remote URL (from `origin`)
    Path       string    // absolute filesystem path to clone
    Plugins    []string  // plugin names discovered under this repo
    CurrentRef string    // short SHA of HEAD
    LastPull   time.Time // mtime of .git/FETCH_HEAD (zero if the file is absent — means never fetched)
}

// Manager owns the set of marketplaces under OrgPluginsDir().
type Manager struct {
    rootDir string // org-plugins directory
}

func NewManager(rootDir string) *Manager

// AddOptions configures Add.
type AddOptions struct {
    // Name overrides the default basename derived from the URL.
    // If set, the clone goes to rootDir/<Name>/ instead of rootDir/<url-basename>/.
    Name string
    // Depth sets git clone depth. 0 = unlimited; default 1 (shallow).
    Depth int
}

// Add clones url into rootDir. Returns the created Repo.
// Errors if the destination basename already exists.
func (m *Manager) Add(ctx context.Context, url string, opts AddOptions) (*Repo, error)

// List returns all known marketplace repos (any directory under rootDir that contains .git).
func (m *Manager) List() ([]Repo, error)

// Get returns a repo by its basename or ErrRepoNotFound.
func (m *Manager) Get(name string) (*Repo, error)

// Update fast-forwards a marketplace. Safe on clean working tree; fails on dirty.
// After a successful pull, the caller is expected to call LinkAll() to resync top-level symlinks.
func (m *Manager) Update(ctx context.Context, name string) error

// UpdateResult is one entry in the return of UpdateAll.
type UpdateResult struct {
    Name       string // repo basename
    Updated    bool   // true if HEAD moved
    FromRef    string // short SHA before pull
    ToRef      string // short SHA after pull
    Err        error  // non-nil if the update failed; other fields may be zero-valued
}

// UpdateAll iterates over List() and updates each. Returns per-repo results.
// Never returns an error itself — per-repo failures are surfaced via UpdateResult.Err.
func (m *Manager) UpdateAll(ctx context.Context) []UpdateResult

// Remove deletes the marketplace clone and any top-level symlinks pointing into it.
func (m *Manager) Remove(name string) error

// LinkAll refreshes top-level symlinks for every marketplace.
// - Discovers plugins under each repo's plugins/ and external_plugins/.
// - Creates/replaces top-level symlink rootDir/<plugin-name> → <repo>/plugins/<plugin-name>.
// - Skips when a top-level entry exists as a real directory (does not overwrite user's own plugins).
// - Reports conflicts via Conflict entries in the return value.
func (m *Manager) LinkAll() (*LinkReport, error)

type LinkReport struct {
    Created    []string // names newly linked
    Updated    []string // names where symlink target changed
    Unchanged  []string
    Conflicts  []Conflict
}

type Conflict struct {
    Name   string
    Reason string // "top-level is a real directory, not overwriting" etc.
}

var ErrRepoNotFound = errors.New("marketplace not found")
```

## Implementation notes

### Git ops via go-git

Use `github.com/go-git/go-git/v5`. Reasons:
- Pure Go → no external `git` dependency
- Works identically across darwin / windows / linux
- Shallow clone support via `Depth: 1` option

Shallow clone (depth 1) by default. Historical commits not required; pull works with shallow clones in modern go-git.

### Plugin discovery

Under each `Repo.Path`:
- Look for `plugins/*/` — each subdir with `.claude-plugin/plugin.json` is a plugin.
- Look for `external_plugins/*/` — same rule, lower precedence (if name collision with `plugins/`, `plugins/` wins within same repo).
- Look for `.claude-plugin/marketplace.json` to validate this repo claims to be a marketplace. If absent, warn but still accept (some repos put plugins directly under `plugins/` without a manifest).

Skip hidden entries (names starting with `.`).

### Symlinking

`os.Symlink(target, link)`. On macOS, relative symlinks are fine but we use **absolute** symlinks (makes `ls -la` output directly traceable).

Replacement logic:
1. If `rootDir/<name>` doesn't exist → `os.Symlink(absTarget, rootDir/<name>)`. Created.
2. If `rootDir/<name>` is a symlink with a different target → `os.Remove` + `os.Symlink`. Updated.
3. If `rootDir/<name>` is a symlink with the same target → no-op. Unchanged.
4. If `rootDir/<name>` is a real directory → do nothing, record Conflict.

### Removal

`Remove(name)` walks top-level entries. For each symlink with target inside `rootDir/<name>`, remove it. Then `os.RemoveAll(rootDir/<name>)`.

### Concurrency

Single-process use expected. No locks. Caller's responsibility not to invoke two `UpdateAll` simultaneously.

## Testing

- Integration tests create a temp directory + init a local bare git repo as a "marketplace" fixture. `Add()` clones from `file://<path>`, `LinkAll()` creates expected symlinks, assertions via `os.Readlink`.
- No network required in tests.
- `TestConflictPreservesRealDirectory`: create `rootDir/foo/` as real dir, verify `LinkAll()` reports conflict and doesn't overwrite.

## Sudo / permissions

On macOS, `/Library/Application Support/Claude/org-plugins/` is typically not writable by a regular user. Manager **does not** attempt `sudo` itself. If `Add` / `LinkAll` hit permission errors, surface them to the CLI layer, which tells the user to re-run with sudo:

```
error: cannot write to /Library/Application Support/Claude/org-plugins: permission denied
hint: re-run with `sudo cowork-mdm marketplace add ...`
```

## Non-goals

- No Windows junction support in v0.2.
- No GPG verification of cloned repos.
- No auto-polling / scheduled updates. User-invoked only.
