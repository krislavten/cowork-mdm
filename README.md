# cowork-mdm

> **Deploy Claude Desktop across enterprise fleets — including the 43 MDM keys outside Anthropic's public docs.** An Electron app ships with 51 managed-preferences keys embedded in its zod schema; the [public enterprise docs](https://support.anthropic.com/en/articles/12188074) cover 8. The other 43 are how you actually point the desktop at Bedrock, Vertex, Foundry, an Anthropic-compatible gateway in front of DeepSeek / Qwen / GLM / MiniMax / Llama / Mistral, or a self-hosted vLLM / SGLang cluster — and lock down egress, MCP, telemetry, sandbox, and auto-update policy while you're at it. `cowork-mdm` pulls the full schema out of the bundle, generates the `.mobileconfig`, lints the payload before it ships, and runs per-host diagnostics afterwards.

**English** · [中文](README.zh.md)

<p align="center">
  <a href="#quickstart"><img alt="Quickstart" src="https://img.shields.io/badge/quickstart-6%20commands-green?style=flat-square" /></a>
  <a href="#claude-code-plugin"><img alt="Claude Code plugin" src="https://img.shields.io/badge/claude%20code%20plugin-5%20skills%20%2B%204%20commands-black?style=flat-square" /></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" /></a>
</p>

**Status**: **v0.3 — CLI + Claude Code plugin.** Schema pinned to Claude.app 1.5354.0. macOS end-to-end; Windows `.reg` tracked in [#9](https://github.com/krislavten/cowork-mdm/issues/9). **Not affiliated with Anthropic** — everything below is reverse-engineered from the public desktop app.

---

## Why this exists

Anthropic's Claude Desktop is an Electron app. Inside the app bundle, alongside the UI code, lives an embedded zod schema that declares **every MDM key the renderer will read at launch**: LLM provider, gateway URL, auth scheme, model allowlist, managed MCP servers, egress allowlist, telemetry endpoint, sandbox constraints, auto-update policy, credential-helper script — **51 keys in total**.

Anthropic's [public enterprise docs](https://support.anthropic.com/en/articles/12188074) cover **8 of them**. The other 43 — the ones you actually need if you don't intend to run Claude Desktop against `api.anthropic.com` — ship only as strings inside a minified Electron chunk.

IT teams hit this wall every time they try to:

- Route the desktop to an **open-weights model family or compatible provider** (DeepSeek, Qwen, Zhipu GLM, MiniMax, Llama, Mistral, and similar) served through an Anthropic-compatible endpoint.
- Route it to a **hyperscaler-managed** Bedrock / Vertex / Foundry deployment of Claude or another model.
- Route it to a **self-hosted vLLM / SGLang** cluster behind the same shim.
- Lock down **egress**, **managed MCP**, **telemetry**, **sandbox**, or **auto-update** policy at fleet scale — none of which is in the public docs.
- Ship **company skills, slash commands, and plugin-bundled MCPs** to every employee Mac, which the mobileconfig channel deliberately refuses to deliver ([evidence](docs/research/skills-plugins-mdm.md)).

`cowork-mdm` is the missing half of the enterprise story: a CLI that extracts the full schema, generates correct MDM profiles against it, lints them before you hand the file to Jamf / Intune / Kandji, and diagnoses the employee Mac when something doesn't take.

## At a glance

| | What you get |
|---|---|
| **Schema** | All 51 managed-preferences keys, extracted verbatim from the app's embedded zod schema (pinned to Claude.app 1.5354.0). Queryable by `cowork-mdm schema list` and `schema show <key>` — name, type, scope, `appMin`, allowed values, example. |
| **Templates (9)** | `gateway` · `gateway-deepseek` · `gateway-glm` · `gateway-minimax` · `bedrock-basic` · `vertex` · `foundry` · `enterprise-cn-full` (one-shot full enterprise scaffold: LLM + MCP + egress + telemetry + sandbox + auto-update) · `mcp-only` |
| **Profile authoring** | `profile show-template NAME --out overrides.yaml` → edit → `profile new --from overrides.yaml --out company.mobileconfig`. `--template` and `--from` are mutually exclusive by design — scaffolds are not production profiles. |
| **Lint as a pre-distribution gate** | `profile validate` is schema-only — it accepts `REPLACE_WITH_YOUR_API_KEY` as a valid string. `profile lint` scans for leftover `REPLACE_*` placeholders and exits non-zero. Run both, not one. |
| **MDM delivery recipes** | Jamf Pro · Microsoft Intune · Kandji — full Custom Settings payload + Shell Script payload recipes in [`docs/deployment-cn.md`](docs/deployment-cn.md). |
| **Plugin delivery (macOS)** | `marketplace add <repo>` clones a Claude-Code-format plugin marketplace into `/Library/Application Support/Claude/org-plugins/` and symlinks each plugin. Designed to run as a Shell Script payload alongside the mobileconfig push. |
| **Per-host diagnostics** | On macOS, `cowork-mdm doctor` runs 9 checks across app install, active plist, org-plugins symlinks, marketplace repo health, user sessions, and git availability — with `--fix` for the ones that are safe to auto-repair. Windows runs a smaller subset until [#9](https://github.com/krislavten/cowork-mdm/issues/9) lands. |
| **Claude Code plugin** | 5 skills + 4 slash commands make the CLI self-driving inside an agent: `/deploy`, `/new-profile`, `/doctor`, `/refresh-plugins`. Ships **zero new logic**; requires the CLI on `PATH`. |
| **Platforms** | macOS (full), Windows (`.reg` encoder tracked in [#9](https://github.com/krislavten/cowork-mdm/issues/9)). |
| **License** | MIT. |

## Quickstart

```bash
brew install krislavten/tap/cowork-mdm

# 6-command happy path for an enterprise gateway deployment:
cowork-mdm profile show-template enterprise-cn-full --out overrides.yaml
$EDITOR overrides.yaml                           # fill every REPLACE_* slot
cowork-mdm profile new --from overrides.yaml \
  --payload-identifier-prefix com.acme.it \
  --out company.mobileconfig
cowork-mdm profile lint company.mobileconfig    # must exit 0 before you ship
cowork-mdm profile validate company.mobileconfig
# Push company.mobileconfig via Jamf / Intune / Kandji.
# For skills + slash commands + plugin-bundled MCPs, add a companion Script
# payload that runs `cowork-mdm marketplace add <your-org-plugins-repo>`.
```

For hyperscaler-managed routes (Bedrock / Vertex / Foundry), substitute `bedrock-basic` / `vertex` / `foundry` for the template name and fill `{{ACCOUNT}}` / region / model-ID placeholders. Same downstream pipeline.

**Full recipe**: [`docs/deployment-cn.md`](docs/deployment-cn.md) — 8 sections covering prerequisites, provider selection, profile generation, lint + validate gates, MDM distribution (Jamf + Intune + Kandji, with Script payload for plugins), employee-machine verification, common failure modes, and the update path.

## Four load-bearing ideas

### 1 · The schema is the source of truth — and it lives in the bundle, not the docs

Claude Desktop is an Electron app. Its embedded zod schema declares every MDM key the renderer reads at launch: name, type, scope, `appMin`, allowed values. We extract that schema verbatim and pin it to a specific Claude.app version (currently **1.5354.0**). `cowork-mdm schema list` and `schema show <key>` surface it; the profile encoder validates against it; the template library draws allowed values from it. Upstream version bumps → re-extract → re-pin → re-ship. No hand-maintained key lists drifting from reality.

### 2 · Templates are YAML sources you own, not opaque scaffolds

Every built-in template ships as hand-written YAML under [`internal/profile/templates/`](internal/profile/templates/). `profile show-template NAME --out overrides.yaml` dumps the exact same YAML into your own config repo — the CLI's embedded copy and the file you edit are byte-identical, so there's no version drift. You edit YAML, we encode `.mobileconfig` / plist / (soon) `.reg`. `--template` and `--from` are mutually exclusive on a single invocation: scaffolds are a starting point, not a deployable artifact.

### 3 · `validate` and `lint` are different gates, and you need both

`profile validate` is schema-only — it accepts `REPLACE_WITH_YOUR_API_KEY` as a valid string because the schema says `inferenceGatewayApiKey: string`. Every enterprise template leaves `REPLACE_*` placeholder tokens at every slot that must be filled before distribution. `profile lint` scans the encoded artifact for those tokens and exits non-zero on any finding. Run validate to catch schema bugs before you push the YAML; run lint as the final pre-distribution gate before you hand the mobileconfig to Jamf. Skipping either will ship a broken deployment that still "works" under one of the two checks.

### 4 · MDM delivers the config layer; a Script payload delivers the rest

The mobileconfig channel — Apple's managed-preferences mechanism — carries **LLM config**, **standalone MCP servers**, **egress**, **telemetry**, and **sandbox** policy. It **cannot** deliver skills, slash commands, hooks, or plugin-bundled MCPs. Those live under `/Library/Application Support/Claude/org-plugins/` and are populated by `cowork-mdm marketplace add <repo>` against a Claude-Code-format plugin marketplace. Enterprise deployment is a two-wave push: **Wave 1** — Custom Settings Payload (mobileconfig). **Wave 2** — Shell Script Payload (`marketplace add` + `marketplace update`). Both scoped to the same device group, on the same cadence. [Reverse-engineered evidence for why this is a hard constraint, not a workaround](docs/research/skills-plugins-mdm.md).

## Command reference

Grouped by intent. Every subcommand accepts `--json` for machine-readable output.

**Author a profile**

```bash
cowork-mdm profile templates                           # list built-in templates (9)
cowork-mdm profile show-template NAME [--out FILE]     # dump template YAML source
cowork-mdm profile new --from overrides.yaml --out out.mobileconfig
cowork-mdm profile lint out.mobileconfig               # flag REPLACE_* placeholders
cowork-mdm profile validate out.mobileconfig           # schema check
```

**Inspect the schema**

```bash
cowork-mdm schema list                          # all 51 keys
cowork-mdm schema show inferenceProvider        # description, example, allowed values
cowork-mdm paths show [--os darwin|windows]     # paths cowork-mdm reads per platform
```

**Apply & verify on a host**

```bash
cowork-mdm profile apply company.mobileconfig --dry-run    # preview, no writes
cowork-mdm profile status                                   # what's active right now
cowork-mdm doctor [--fix]                                   # diagnose a broken install
```

**Manage org plugins (macOS)**

```bash
cowork-mdm marketplace add https://github.com/<org>/claude-org-plugins
cowork-mdm marketplace update
cowork-mdm plugin list
cowork-mdm plugin prune
```

Spec documents live under [`specs/`](specs/); the full v0.2 task breakdown is at [`docs/execution/TASKS.md`](docs/execution/TASKS.md).

## Claude Code plugin

v0.3 ships a Claude Code plugin layer so an agent can drive the CLI on your behalf.

- **Skills (5)**: `cowork-mdm`, `mdm-profile-authoring`, `mdm-profile-deploy`, `mdm-plugins`, `mdm-doctor`.
- **Slash commands (4)**: `/deploy`, `/new-profile`, `/doctor`, `/refresh-plugins`.
- **Zero new logic.** Every skill shells out to the `cowork-mdm` CLI on `PATH`.

Install:

```
/plugin marketplace add https://github.com/krislavten/cowork-mdm
/plugin install cowork-mdm@cowork-mdm
```

Full surface documented in [`specs/claude-plugin.md`](specs/claude-plugin.md).

## Deployment flow

```
         ┌─────────────────────────────────────┐
         │  Claude Desktop 1.5354.0 (Electron) │
         │  embedded zod schema: 51 keys       │
         └────────────────┬────────────────────┘
                          │ extracted + pinned into internal/schema/schema.json
                          ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  cowork-mdm CLI (Go 1.23, single static binary)              │
  │    schema ─────→ list / show / paths                         │
  │    profile ────→ templates / show-template / new             │
  │                  lint / validate / apply --dry-run / status  │
  │    marketplace → add / update / remove                       │
  │    plugin ─────→ list / prune                                │
  │    doctor ─────→ per-host diagnostics (9 checks on macOS)    │
  └─────────┬──────────────────────────────────┬─────────────────┘
            ▼                                  ▼
  ┌──────────────────────┐          ┌──────────────────────────┐
  │  .mobileconfig       │          │  org-plugins/ directory  │
  │  (Custom Settings)   │          │  (Shell Script payload)  │
  │  LLM · MCP · egress  │          │  skills · slash cmds     │
  │  telemetry · sandbox │          │  plugin-bundled MCPs     │
  └─────────┬────────────┘          └─────────┬────────────────┘
            │                                 │
            └───────────┬─────────────────────┘
                        ▼
           ┌──────────────────────────────────┐
           │  Jamf Pro · Intune · Kandji      │
           │  two-wave push to employee Macs  │
           └──────────────────────────────────┘
```

## Contributing

Development conventions in [AGENTS.md](AGENTS.md). Specs in [`specs/`](specs/). Issues and PRs welcome.

## Maintainer notes

### Releasing

Releases are tag-triggered. Push a `v*` tag and `.github/workflows/release.yml` runs GoReleaser.

The release job publishes to two places:

1. **GitHub Releases** on this repo — uses the default `GITHUB_TOKEN`.
2. **Homebrew tap** at `krislavten/homebrew-tap` — requires secret `HOMEBREW_TAP_GITHUB_TOKEN` with **contents:write** on that repo. Without it, the brew formula push step fails; everything else (GitHub Release + binaries + checksums) still succeeds.

Set the secret once:

```bash
gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo krislavten/cowork-mdm
# paste the PAT when prompted
```

## License

MIT — see [LICENSE](LICENSE).
