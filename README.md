# cowork-mdm

> CLI toolkit for deploying Claude Desktop in the enterprise: MDM config profiles, plugin marketplace management, and per-host diagnostics.

**Status**: **v0.3 — CLI + Claude Code plugin.** The CLI delivers profile generation, plugin marketplace management, and diagnostics. v0.3 adds a Claude Code plugin layer (skills + slash commands) that teaches an agent how to drive the CLI on the user's behalf. Plugin `v0.3.0` wraps CLI `v0.3.0`; both ship together.

**Not affiliated with Anthropic.** This project is an independent effort based on reverse-engineering the public Claude Desktop application.

## Why

Anthropic's public enterprise documentation covers **8 of the 51** MDM keys that `Claude.app` actually reads. The remaining keys — `inferenceProvider`, `inferenceBedrockRegion`, `managedMcpServers`, `coworkEgressAllowedHosts`, `bootstrapUrl`, and more — are defined in the app's embedded zod schema (`FJ = me({...})`) but undocumented publicly.

Deploying Claude Desktop in 3rd-party inference mode (Bedrock, Vertex, LLM gateway, Azure Foundry) relies heavily on these undocumented keys. `cowork-mdm` surfaces the schema, generates correct config profiles (`.mobileconfig` / `.reg` / Jamf / Intune formats), manages the org plugin marketplace, and runs per-host diagnostics — so IT admins don't have to reverse-engineer the Electron bundle themselves.

## Quick start

```bash
# macOS (Homebrew)
brew install krislavten/tap/cowork-mdm

# Or download a binary from the Releases page:
# https://github.com/krislavten/cowork-mdm/releases
```

## Commands

```bash
# Schema + path reference
cowork-mdm schema list                     # all 51 keys (name, type, scope, appMin)
cowork-mdm schema show inferenceProvider   # details: description, example, allowed values
cowork-mdm paths show                      # host paths cowork-mdm reads
cowork-mdm paths show --os windows         # simulate a different platform

# Profile authoring (YAML → .mobileconfig / plist)
# --template and --from are mutually exclusive; pick one:
cowork-mdm profile templates
cowork-mdm profile new --template bedrock-basic --out my.mobileconfig  # built-in verbatim
cowork-mdm profile new --from overrides.yaml --out my.mobileconfig     # your own YAML
cowork-mdm profile validate my.mobileconfig
cowork-mdm profile status                  # what's currently active on this host

# Marketplace + plugin management (macOS)
cowork-mdm marketplace add https://github.com/anthropics/claude-plugins-official
cowork-mdm marketplace update
cowork-mdm plugin list
cowork-mdm plugin prune

# Diagnostics
cowork-mdm doctor
cowork-mdm doctor --fix
```

Every subcommand accepts `--json` for machine-readable output. Spec and
task breakdown: [`specs/`](specs/) + [`docs/execution/TASKS.md`](docs/execution/TASKS.md).

## Enterprise deployment

**Chinese LLM providers (DeepSeek / GLM / MiniMax) + MDM fleet rollout**:
see [docs/deployment-cn.md](docs/deployment-cn.md) for the full cookbook
covering Jamf / Intune / Kandji distribution, the `enterprise-cn-full`
template, and the Script-payload pattern for shipping company plugins
alongside the mobileconfig.

## Use as a Claude Code plugin

v0.3 adds a Claude Code plugin layer: five skills + four slash commands
that teach an agent how to drive the `cowork-mdm` CLI safely on a user's
behalf. The plugin ships **no new logic** — it's a documentation bundle
that makes the CLI self-driving inside Claude Code.

### Install (in Claude Code)

```
/plugin marketplace add https://github.com/krislavten/cowork-mdm
/plugin install cowork-mdm@cowork-mdm
```

The CLI itself must still be installed via Homebrew (see Quick start) —
the plugin reports "CLI missing, install via brew" and stops if `cowork-mdm`
isn't on `PATH`.

### What the agent gets

**Skills** — loaded automatically when the user's request matches:

| Skill | Loaded when the user asks about … |
| --- | --- |
| `cowork-mdm` | generic Claude Desktop MDM questions (routes to a specialist) |
| `mdm-profile-authoring` | writing / editing a profile, looking up schema keys, Bedrock / Vertex / Azure / gateway recipes |
| `mdm-profile-deploy` | pushing a profile via Jamf / Intune / Kandji, verifying status, why a config isn't taking effect |
| `mdm-plugins` | installing or updating `org-plugins/` marketplaces, dangling symlinks |
| `mdm-doctor` | troubleshooting a broken Claude Desktop install |

**Slash commands** — executable playbooks:

| Command | What it does |
| --- | --- |
| `/cowork-mdm:new-profile` | interactive profile generator (pick provider → collect values → generate → validate) |
| `/cowork-mdm:deploy PATH` | validate + dry-run preview, diff against current host, hand off for MDM push |
| `/cowork-mdm:doctor` | run `cowork-mdm doctor --json`, interpret findings, suggest specific fixes |
| `/cowork-mdm:refresh-plugins` | `marketplace update` + `plugin prune` dry-run |

### Safety invariants the plugin enforces

- **Never `sudo`.** Writes to `/Library/Managed Preferences/` and
  `/Library/Application Support/Claude/` always remain the user's explicit
  action.
- **Production deploys go through an MDM channel.** Direct plist writes
  are clobbered by `managedappconfigd` on the next sync; the plugin
  refuses that path.
- **Enterprise secrets stay in the user's overrides YAML**, never in the
  repo's built-in templates.

## Contributing

See [AGENTS.md](AGENTS.md) for development conventions. Issues and PRs welcome.

## Maintainer notes

### Releasing

Releases are tag-triggered. Push a `v*` tag and `.github/workflows/release.yml` runs GoReleaser.

The release job publishes to two places:
1. **GitHub Releases** on this repo — uses the default `GITHUB_TOKEN`.
2. **Homebrew tap** at `krislavten/homebrew-tap` — requires secret `HOMEBREW_TAP_GITHUB_TOKEN` on this repo. The token must be a personal access token (classic or fine-grained) with **contents:write** on `krislavten/homebrew-tap`. Without it, the brew formula push step fails; everything else (GitHub Release + binaries + checksums) still succeeds.

Set the secret once:
```bash
gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo krislavten/cowork-mdm
# paste the PAT when prompted
```

## License

MIT — see [LICENSE](LICENSE).
