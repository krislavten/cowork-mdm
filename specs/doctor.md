# Spec: `internal/doctor/`

## Intent

Enumerate environment checks that tell a sysadmin "is Claude Desktop deployment healthy on this host?". Each check is a pure function producing a result; orchestrator collects all and optionally attempts fixes.

## Public interface

```go
package doctor

type Status int

const (
    StatusOK       Status = iota // Green
    StatusWarning                // Yellow (non-critical)
    StatusError                  // Red (must-fix)
    StatusSkipped                // N/A on this platform or precondition unmet
)

type Result struct {
    ID          string   // stable id for JSON output, e.g. "plist.syntax"
    Name        string   // human title
    Status      Status
    Message     string   // short summary
    Detail      string   // multi-line detail, shown in verbose mode
    FixAvailable bool
}

// Check performs one check. Fixable checks also expose a Fix method.
type Check interface {
    Run(ctx context.Context) Result
}

type Fixable interface {
    Check
    Fix(ctx context.Context) error
}

// Runner orchestrates checks.
type Runner struct {
    Checks []Check
}

// DefaultRunner returns a Runner with all checks appropriate for the current platform.
func DefaultRunner() *Runner

// Run executes all checks in order. Returns all results; does not stop on failure.
func (r *Runner) Run(ctx context.Context) []Result

// RunAndFix executes all checks. For each non-OK Fixable, calls Fix and re-runs that check.
// Returns final results (after fix attempts).
func (r *Runner) RunAndFix(ctx context.Context) []Result

// JSON produces machine-readable output.
func JSON(results []Result) ([]byte, error)

// Exit returns an exit code:
//   0 — all OK or Warning only
//   1 — any Error
//   2 — internal doctor failure
func Exit(results []Result) int
```

## Check inventory (macOS)

Each check is a struct implementing `Check`:

| ID | Name | Fix |
|---|---|---|
| `app.installed` | Claude.app present at /Applications/Claude.app | no |
| `app.version` | Claude.app Info.plist version within supported range | no |
| `plist.exists` | /Library/Managed Preferences/com.anthropic.claudefordesktop.plist exists | no |
| `plist.syntax` | Plist parses | no |
| `plist.schema` | All keys match schema types, no unknowns | no |
| `plist.provider-consistency` | If inferenceProvider set, corresponding provider-specific keys present | no |
| `orgplugins.exists` | /Library/Application Support/Claude/org-plugins exists | yes (mkdir) |
| `orgplugins.readable` | user can `readdir` (warn if only root can, since cowork-mdm needs to read) | no |
| `orgplugins.symlinks.resolved` | every top-level symlink resolves | yes (prune) |
| `orgplugins.symlinks.targets-in-org` | each symlink points inside org-plugins | no |
| `marketplace.clean` | every repo under org-plugins has clean working tree | no |
| `aws.credentials` | if inferenceProvider=bedrock: ~/.aws/credentials has the declared profile | no |
| `aws.region-match` | if bedrock: configured region ∈ known Bedrock regions | no |
| `user.sessions.discoverable` | Claude-3p sessions dir exists (warn — means user never launched app) | no |
| `user.sessions.enabledplugins` | enabledPlugins consistency across sessions | no |
| `marketplace.repos-operable` | every marketplace repo under org-plugins/ has a parseable `.git` and valid HEAD via go-git | no |

## Check inventory (Windows)

| ID | Name | Fix |
|---|---|---|
| `app.installed` | Claude.exe at expected install location | no |
| `app.version` | from file metadata / registry | no |
| `registry.exists` | HKLM\SOFTWARE\Policies\Claude exists | no |
| `registry.schema` | values match schema types, no unknowns | no |

## JSON output

```json
{
  "summary": {
    "total": 16,
    "ok": 12,
    "warning": 2,
    "error": 2,
    "skipped": 0
  },
  "results": [
    {
      "id": "plist.schema",
      "name": "Managed plist matches schema",
      "status": "error",
      "message": "Unknown key 'fooBar' not in 51-key schema",
      "detail": "Key fooBar first appeared at line 17. Either remove or update to a known key.",
      "fixAvailable": false
    }
  ]
}
```

## Testing

- Each check has a `*_test.go` with table-driven scenarios using fixture plists / registry dumps.
- Orchestrator integration test uses a synthetic Runner with mock checks.
- Platform-gated: `checks_darwin_test.go` runs only on macOS; a cross-platform `runner_test.go` uses in-memory mock checks.

## Non-goals

- No network checks (reaching anthropic.com etc.). Kept offline.
- No auto-fix for misconfigured provider keys (complex, user-specific).
- No Linux checks.
