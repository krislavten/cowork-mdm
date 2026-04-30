# Spec: `cmd-wiring/` + `cmd/cowork-mdm/main.go`

## Intent

Translate user CLI invocations to calls into internal packages. No business logic in this layer — only argument parsing, output formatting, and error reporting.

## Command surface

Invocation: `cowork-mdm [global-flags] <command> [subcommand] [flags] [args]`.

### Global flags

```
--help, -h        Show help
--version         Print version
--json            Machine-readable output (where applicable)
--verbose, -v     Show debug info
--no-color        Disable ANSI color
```

### `cowork-mdm schema`

```
cowork-mdm schema list
  Lists every key in the embedded schema as a table:
  NAME | TYPE | SCOPES | APP-MIN | DESCRIPTION (truncated)
  --json emits array of Key objects.

cowork-mdm schema show <key-name>
  Prints full details for one key: name, type, scopes, appMin, title,
  description, example, legacyAlias, sensitive.
  Exits non-zero if key-name unknown.

cowork-mdm schema extract --from <path-to-claude.app> [--out schema.json]
  Maintainer tool. Extracts FJ schema from the given Claude.app.
  Emits JSON. Default out: stdout. Build-tagged — only available in dev builds.
```

### `cowork-mdm profile`

```
cowork-mdm profile new [--template NAME] [--out FILE]
  With --template: non-interactive, fills template defaults, writes to --out.
  Without --template: opens TUI wizard.
  Default --out: stdout.

cowork-mdm profile validate FILE
  Parses FILE (auto-detect by extension), validates every key against schema.
  Exits 0 if valid, non-zero with detail otherwise.

cowork-mdm profile diff OLD NEW
  Parses both, compares key-by-key.
  Output: unified diff-style with +key=value / -key=value.

cowork-mdm profile export FILE --format {mobileconfig|plist|jamf|intune|reg}
  Parses FILE, re-emits as specified format. Writes to stdout.

cowork-mdm profile apply FILE [--dry-run]
  Thin orchestrator: parses FILE into a Profile, then delegates to
  internal/managed.Apply(). Does NOT perform filesystem or registry writes
  itself. Formats and exits based on managed.ApplyResult + managed.ErrPermission.
  Requires admin/root at runtime.

cowork-mdm profile status
  Thin orchestrator over internal/managed.Status(). Reports:
    - target path / registry key
    - whether the managed config is present
    - decoded key count + unknown keys
    - any parse error
    - --json: full StatusReport dump
```

### `cowork-mdm marketplace` (macOS only)

```
cowork-mdm marketplace add URL [--name NAME]
  Clones the git repo to org-plugins/<basename> (or --name).
  Auto-runs LinkAll after successful clone.

cowork-mdm marketplace list
  Table: NAME | URL | HEAD-SHORT | PLUGINS-COUNT | LAST-PULL.
  --json emits array of Repo.

cowork-mdm marketplace update [NAME]
  Without NAME: updates all.
  After pull, runs LinkAll. Reports Created/Updated/Conflicts.

cowork-mdm marketplace remove NAME
  Removes clone + associated symlinks. Confirms via prompt unless --yes.
```

### `cowork-mdm plugin` (macOS only)

```
cowork-mdm plugin list
  Table: NAME | SOURCE | TARGET | STATUS (ok / dangling / real-dir).
  --json emits array of Plugin.

cowork-mdm plugin show NAME
  Full details: source, target path, manifest contents, enabled state across user sessions.

cowork-mdm plugin unlink NAME
  Removes top-level symlink. Refuses to operate on real directories.

cowork-mdm plugin prune
  Removes dangling symlinks. Dry-run by default; pass --yes to commit.
```

### `cowork-mdm doctor`

```
cowork-mdm doctor
  Runs all platform-appropriate checks. Prints table:
  [STATUS] ID — MESSAGE
  Exit codes: 0 (all green or warnings only) / 1 (any error) / 2 (doctor internal failure).

cowork-mdm doctor --fix
  Attempts automatic fixes for any non-OK Fixable check.

cowork-mdm doctor --json
  Emits full Result array as JSON.

cowork-mdm doctor --verbose
  Includes Detail field in output.
```

## Exit codes

Consistent across all commands:

- `0` — success
- `1` — validation failure / check error / operation failed
- `2` — argument / flag error
- `3` — permission denied (needs sudo/admin)
- `4` — not supported on this platform

## Output formats

### Human (default)

Tables use tab-aligned columns. Colors:
- Green: StatusOK
- Yellow: StatusWarning
- Red: StatusError
- Dim: StatusSkipped

Disable with `--no-color` or when `!isatty(stdout)`.

### JSON

`--json` flag (where supported). Always newline-terminated. No human formatting. Schema documented in each command's help.

## Error messages

- Short form for expected errors: `profile validate: unknown key "fooBar"`
- Long form for unexpected: stack trace to stderr, short user-facing message to stdout.
- Suggest next step when possible: `hint: re-run with sudo`

## Testing

- `cmd-wiring/*_test.go` uses `cobra.Command.SetOut()` + `SetErr()` to capture output. Runs real internal packages against fixtures — not mocks.
- `testing/integration/` compiles the binary and runs it in a subprocess. Used for end-to-end smoke (e.g., `cowork-mdm schema list | grep inferenceProvider`).

## Non-goals

- No shell completions in v0.2 (v0.3).
- No config file (all flags, no `.cowork-mdmrc`).
