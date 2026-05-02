---
name: mdm-doctor
description: Diagnosing a broken Claude Desktop install — the managed profile isn't being read, plugins aren't showing up, the app won't launch, or the user's config differs from what the admin pushed. Load when the user is troubleshooting rather than setting up — phrases like "Claude Desktop won't launch", "doctor", "diagnose this host", "why isn't my profile active", "debug Claude", "something's broken with Claude Desktop". Uses `cowork-mdm doctor` to enumerate host state.
---

# MDM doctor

This skill covers **diagnosis**. The entry point is `cowork-mdm doctor`,
which enumerates every thing that can be wrong with a Claude Desktop
install and returns a structured result per check.

## The command in three forms

```bash
cowork-mdm doctor              # human table; exit non-zero on any error
cowork-mdm doctor --json       # structured; parse this when reasoning
cowork-mdm doctor --fix        # attempt auto-fix on any fixAvailable non-OK check
```

**Always start with `doctor --json`**, parse it, then decide what to do.
Don't chain to `--fix` blindly — some fixes modify the filesystem and the
user may not want them.

## JSON result shape

```json
{
  "platform": "darwin",
  "results": [
    {
      "id": "app.installed",
      "name": "Claude Desktop installed",
      "status": "ok" | "warning" | "error" | "skipped",
      "message": "/Applications/Claude.app",
      "detail": "…",
      "fixAvailable": false
    }
  ],
  "summary": {"ok": 9, "total": 9}
}
```

Key fields to parse:

- `results[].id` — dotted identifier (e.g. `app.installed`, `plist.exists`,
  `plist.schema`, `orgplugins.exists`, `orgplugins.symlinks`,
  `orgplugins.dangling`, `marketplace.repos-operable`, `user.sessions`,
  `git.available`).
- `results[].status` — one of `ok` / `warning` / `error` / `skipped`.
- `results[].message` — short human string; **may be absent** on `ok` or
  `skipped` results (omitempty). Guard your access.
- `results[].detail` — optional longer explanation; **omitempty**, often
  absent. Only read it after checking existence.
- `results[].fixAvailable` — boolean; `--fix` only attempts to repair
  checks where this is true.
- `summary` — counts by status. Fast path: if `summary.ok == summary.total`
  the host is clean; otherwise walk `results` for the non-OK entries.

Status semantics:

- `skipped` — check doesn't apply on this OS (e.g. registry checks on
  macOS). Not a problem.
- `warning` — non-blocking anomaly. Usually informational.
- `error` — exit-1 condition. At least one means Claude Desktop almost
  certainly has a functional issue.

## Decision tree by finding

### `app.installed` error

Claude Desktop isn't installed (or is in an unexpected location). Fix:
user installs from the download page. No amount of plugin/profile work
matters until the app is on disk.

### `plist.exists` error on macOS

No managed plist found at either system or user path.
- If the user **expected** a managed profile → MDM push never landed, or
  landed under the wrong bundle identifier. Switch to `mdm-profile-deploy`.
- If the user **didn't** push a profile → this is expected; the app falls
  back to 1p mode. Not a bug.

### `plist.schema` error

The plist is there but malformed (broken XML, wrong encoding). This is a
sign of hand-editing gone wrong or a truncated MDM push, OR a schema-
invalid plist (unknown keys, wrong types). Possible causes:
- **Unknown keys** — newer app than CLI, or a hand-typed typo like
  `inferenceBedrockRegionn`. Check the check's `detail` field for the
  offending key name, then `cowork-mdm schema list --json` for canonical
  names.
- **Malformed XML** — `plutil -lint <path>` to see the XML error.
  Regenerate via MDM push, not by hand.

### `orgplugins.exists` error

`/Library/Application Support/Claude/org-plugins/` is missing. Claude
Desktop creates it on first launch, so this error usually means the user
hasn't launched the app yet, or they're on a fresh install. `--fix`
creates the directory.

### `orgplugins.symlinks` warning

Top-level entries exist that aren't valid (pointing outside the managed
marketplaces). Likely hand-installed plugins. Not broken, just
non-standard.

### `orgplugins.dangling` warning

Dangling symlinks present. These log warnings at every Claude launch. Fix:
`cowork-mdm plugin prune --yes` (it's `fixAvailable: true` — `--fix` does
this).

### `user.sessions` warning

No user session DB found. Means no one has actually launched Claude
Desktop on this Mac since install. Not a problem if the user knows that.

### `marketplace.repos-operable` warning

Inconsistency between `org-plugins/<name>/` directories and the top-level
symlinks. Usually from a half-completed `marketplace remove`. Fix:
`cowork-mdm marketplace update` rebuilds links.

### `git.available` error

`git` isn't on PATH or isn't callable. `marketplace add/update` uses
go-git internally so it works without a system git, but the doctor still
reports this as it makes manual debugging harder.

## The `--fix` contract

`--fix` runs checks whose JSON shows `fixAvailable: true` and attempts
remediation. What it can fix today:

- Create missing `org-plugins/` directory.
- Prune dangling top-level symlinks.

What it will **not** fix:

- Anything that requires writing to `/Library/Managed Preferences/` —
  `mdm-profile-deploy` rule 1 applies.
- Missing Claude.app install.
- Plist schema errors (refuses to rewrite potentially hand-tuned content).

Before running `--fix`, tell the user exactly which checks will be
affected. `--fix --json` prints the results after attempted fixes.

## Reading `cowork-mdm paths show` for context

```bash
cowork-mdm paths show --json
```

Returns the exact paths `cowork-mdm` is consulting. The JSON uses
**PascalCase** field names — `ClaudeAppPath`, `ManagedPrefsPlist`,
`ManagedPrefsUserPlist(<you>)`, `OrgPluginsDir`, `UserSessionsDir`,
`LaunchAgentDir`, `WindowsRegistryPath`. Parse with those exact keys.

**Watch out**: the key `ManagedPrefsUserPlist(<you>)` contains a literal
`<you>` placeholder — the CLI does not substitute the username into the
key name itself (only into the value). Don't regex-rewrite the key; read
it verbatim. Example jq:

```bash
cowork-mdm paths show --json | jq '.["ManagedPrefsUserPlist(<you>)"]'
```

If the user has customized any of these (e.g. Parallels/VDI installs where
`/Library/Application Support/Claude/` is bind-mounted elsewhere), the
doctor checks against the defaults and will report errors even when the
app is functional. Suspect this when the user swears the config works.

## Suggested diagnostic flow

```
1. cowork-mdm paths show --json            # confirm where we're looking
2. cowork-mdm doctor --json                # enumerate findings
3. For each error/warning, consult the decision tree above
4. Propose specific commands, don't run --fix blindly
5. For profile-specific issues, hand off to mdm-profile-deploy
6. For plugin-specific issues, hand off to mdm-plugins
```

## What doctor does NOT check

- Whether `inferenceModels` ARNs are valid / the user has Bedrock
  entitlement — that's an AWS-side question.
- Whether MCP servers in `managedMcpServers` are reachable — ping them
  yourself.
- Whether the user's Claude Desktop version satisfies each key's
  `appMin` — you can cross-check with `schema show KEY` and
  `/Applications/Claude.app/Contents/Info.plist`.
