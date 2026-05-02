# Spec: `plugin/` — Claude Code plugin surface (v0.3)

## Intent

Ship `cowork-mdm` not just as a CLI binary but also as a **Claude Code plugin**
so that an agent running inside Claude Code (Desktop or Code CLI) can drive the
CLI on the user's behalf: generate a profile, validate it, install org plugins,
diagnose a broken host.

The plugin ships **no code** — it is a bundle of text assets (skills, slash
commands, permission grants) that teaches an agent:

1. what `cowork-mdm` is and when it applies,
2. which subcommand to reach for in which situation,
3. the safe sequencing of read → validate → (user-mediated) apply,
4. the gotchas that cannot be learned from `--help` alone (e.g. the
   `managedappconfigd` clobber trap).

CLI remains the single source of truth for behavior. Plugin assets reference
CLI subcommands by name and rely on `--json` for machine-readable output.

## Non-goals

- No MCP server, no custom tool protocol. The agent drives the CLI via Bash.
- No substitute for `--help`. Skills describe **workflows**, not flag tables.
- The plugin does not issue `sudo` on the user's behalf. Apply always stays
  manual or MDM-channel mediated — the skill explains why.

## Directory layout

```
.claude-plugin/
  marketplace.json            # declares this repo as a single-plugin marketplace
plugin/
  .claude-plugin/
    plugin.json               # plugin manifest + permission grants
  skills/
    cowork-mdm/SKILL.md              # thin router / index
    mdm-profile-authoring/SKILL.md
    mdm-profile-deploy/SKILL.md
    mdm-plugins/SKILL.md
    mdm-doctor/SKILL.md
  commands/
    new-profile.md
    deploy.md
    doctor.md
    refresh-plugins.md
```

## `marketplace.json` (repo root)

```json
{
  "name": "cowork-mdm",
  "owner": { "name": "krislavten" },
  "plugins": [
    {
      "name": "cowork-mdm",
      "source": "./plugin",
      "description": "Claude Desktop MDM toolkit — profile authoring, plugin marketplace, doctor."
    }
  ]
}
```

Users install with:

```
/plugin marketplace add https://github.com/krislavten/cowork-mdm
/plugin install cowork-mdm@cowork-mdm
```

## `plugin.json` (plugin root)

```json
{
  "name": "cowork-mdm",
  "version": "0.3.0",
  "description": "Claude Desktop MDM: profile authoring, plugin marketplace, doctor. Requires the `cowork-mdm` CLI on PATH.",
  "author": { "name": "krislavten" },
  "homepage": "https://github.com/krislavten/cowork-mdm"
}
```

- `version` must match the CLI release tag (`v0.3.0`).
- Permission grants (`Bash(cowork-mdm:*)`) are declared per slash command
  via the `allowed-tools:` frontmatter — the same pattern Anthropic's
  `commit-commands` plugin uses for git. The top-level `plugin.json` has
  no `permissions` block; adding one would override the per-command
  scoping.
- The CLI's own permission model (sudo for apply, sudo for writes into
  `/Library/Application Support/Claude/`) is unchanged by any grant here
  — the user still has to run `sudo` themselves for those operations.

## Skills

Five skills, each with `description` written as *"load me when …"* rather than
*"I am …"* — Claude Code selects skills by matching the user request against
the description.

### `cowork-mdm` (router)

- **When loaded**: user mentions "cowork-mdm", "Claude Desktop MDM", "managed
  preferences for Claude", or asks a generic question before the sub-domain is
  clear.
- **Purpose**: 30-line index. Lists the CLI surface and routes the agent to
  one of the four specialist skills.

### `mdm-profile-authoring`

- **When loaded**: user wants to *create* or *edit* an MDM profile — phrases
  like "write a mobileconfig", "configure Bedrock / Vertex / Azure", "lock
  down MCP servers", "pick a provider", "look up a schema key".
- **Purpose**: the agent-facing manual for `cowork-mdm schema` + `cowork-mdm
  profile new/validate`. Covers the 51-key landscape, the three inference
  provider recipes, and the `--from overrides.yaml` pattern for enterprise
  secrets. Format conversions happen via `profile new --format` — there is
  no `profile export` subcommand.

### `mdm-profile-deploy`

- **When loaded**: user wants to *push* a profile to machines — phrases like
  "apply this profile", "push via Jamf/Intune/Kandji", "why isn't my config
  taking effect", "profile status".
- **Purpose**: covers the `apply` / `status` lifecycle and enshrines the
  single most important gotcha: **writing directly to
  `/Library/Managed Preferences/` is clobbered by `managedappconfigd` on the
  next MDM sync**. Production deployments must use an MDM channel. Agent must
  not `sudo` apply on user's behalf.

### `mdm-plugins`

- **When loaded**: user wants to manage Claude Desktop's `org-plugins/`
  directory — "install a plugin", "add a marketplace", "plugin not showing
  up", "dangling symlink".
- **Purpose**: the agent-facing manual for `cowork-mdm marketplace` +
  `cowork-mdm plugin`. Explains the symlink model, the `marketplace add →
  update → link-all → plugin list` cycle, and when `prune` is safe.

### `mdm-doctor`

- **When loaded**: user is troubleshooting a broken Claude Desktop install or
  unclear config state — "Claude won't launch", "doctor", "why isn't my
  profile active", "diagnose this host".
- **Purpose**: reverse workflow (symptom → probable cause → command). Reads
  `cowork-mdm doctor --json`, interprets each check, proposes the next step.

## Slash commands

Slash commands are **executable playbooks** — they tell the agent what to do
in what order, not what the CLI flags are. The CLI is the source of truth for
flags; the command body calls out to CLI subcommands.

### `/cowork-mdm:new-profile`

Interactive: ask the user which inference provider (Bedrock / Vertex / Azure /
Gateway / MCP-only), use the matching built-in template as a structural
reference, prompt the user for enterprise-specific values, author an
`overrides.yaml` in that shape, then run `cowork-mdm profile new
--from overrides.yaml --out out.mobileconfig` — without `--template`, since
the two flags are mutually exclusive. Finally run `profile validate` on
the output. Format conversions go through `profile new --format`; there
is no `profile export` subcommand.

### `/cowork-mdm:deploy PATH`

Validate → show decoded key table → dry-run `profile apply --dry-run` to
preview the plist → **stop and hand off** with instructions for MDM-channel
deployment. Never invokes sudo.

### `/cowork-mdm:doctor`

Run `cowork-mdm doctor --json`, parse, group findings by severity, suggest
`--fix` only for checks whose JSON result has `fixAvailable: true`,
explain each warning.

### `/cowork-mdm:refresh-plugins`

`marketplace update` (all repos) → `plugin list --json` → show diff from the
previous state → `plugin prune` dry-run → ask before committing the prune.

## Testing

End-to-end acceptance test: on the maintainer's own Mac, run
`/cowork-mdm:new-profile` against the live `/Library/Managed Preferences/...`
plist and confirm the generated plist round-trips to the same key set (modulo
whitespace). This is the v0.3 gate — if the plugin can't reproduce the
maintainer's own config, it won't work on customer hosts either.

## Release

- Plugin version field in `plugin.json` and `marketplace.json` entries tracks
  the CLI release tag (both bumped to `0.3.0` at release time).
- No separate CI workflow — plugin assets ship in the repo root; installing is
  `git clone`-based.
