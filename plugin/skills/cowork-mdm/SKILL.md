---
name: cowork-mdm
description: Router for Claude Desktop enterprise administration — loads when the user mentions cowork-mdm by name or asks a generic question about Managed Preferences for Claude Desktop that doesn't clearly belong to profile authoring, deployment, plugins, or diagnostics. Hands off to a specialist sub-skill. Prefer the specialists directly when the user's intent is specific (writing a profile, deploying one, managing org plugins, or troubleshooting a host).
---

# cowork-mdm — router

`cowork-mdm` is a Go CLI that generates and inspects Managed Preferences config
for Claude Desktop. It also manages the org plugin marketplace and diagnoses
hosts. The plugin version ships **no new logic** — every command is a thin
wrapper around `cowork-mdm` invocations.

## Pre-flight: the CLI must be on PATH

```bash
cowork-mdm --version
```

Expected output is a single line starting with `cowork-mdm version`,
followed by either a released tag (e.g. `0.3.0 (commit abc1234, built
2026-05-01)`) **or** the string `dev (commit none, built unknown)` on a
developer-built binary. Both are valid — the plugin works against either.
Only treat this as broken if the command is not found or exits non-zero.

If it fails with "command not found" or equivalent, stop and tell the
user to install it:

```bash
brew install krislavten/tap/cowork-mdm
# or download a binary from https://github.com/krislavten/cowork-mdm/releases
```

Do not try to work around a missing CLI. The plugin cannot do anything
useful without it.

## Pick a sub-skill by task

| User wants to … | Load sub-skill | Typical commands |
| --- | --- | --- |
| Look up a Managed Preferences key, draft / edit a profile for Bedrock/Vertex/Azure/Gateway/MCP-only | **mdm-profile-authoring** | `schema list`, `schema show KEY`, `profile templates`, `profile new --template NAME` OR `profile new --from overrides.yaml` (mutually exclusive), `profile validate` |
| Push an existing profile to real hosts; investigate why it isn't taking effect | **mdm-profile-deploy** | `profile apply --dry-run`, `profile status` |
| Install / update / remove org plugin marketplaces; clean dangling symlinks | **mdm-plugins** | `marketplace add/list/update/remove`, `plugin list/show/prune` |
| Diagnose a specific user's broken Claude Desktop install | **mdm-doctor** | `paths show`, `doctor`, `doctor --fix`, `doctor --json` |

## Slash commands you can offer

- `/cowork-mdm:new-profile` — interactive profile generator.
- `/cowork-mdm:deploy PATH` — validate + preview a profile before MDM push.
- `/cowork-mdm:doctor` — run diagnostics + explain each finding.
- `/cowork-mdm:refresh-plugins` — update all marketplaces + clean symlinks.

## Two ground rules (both apply to every sub-skill)

1. **Prefer `--json` whenever you need to reason about output.** Parse the
   JSON, then narrate. The human-formatted tables drift; the JSON contract
   is stable.
2. **Never `sudo`, never write to `/Library/Managed Preferences/` directly.**
   Production deployments go through an MDM channel (Jamf / Intune / Kandji /
   Mosyle). `profile apply` without `--dry-run` is a direct write and will be
   clobbered by macOS's `managedappconfigd` on the next MDM sync. If the user
   insists on a local test, hand them the `sudo` command, don't run it.

Past those two rules, defer to the specialist skill.
