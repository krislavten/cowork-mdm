# cowork-mdm

**English** · [中文](README.zh.md)

> Deploy Claude Desktop across enterprise fleets — MDM profiles for Bedrock / Vertex / Foundry / third-party and self-hosted LLM gateways, org-plugin distribution, per-host diagnostics.

**Status**: **v0.3 — CLI + Claude Code plugin.** The CLI generates MDM profiles (`.mobileconfig` / plist), manages the org plugin marketplace, and diagnoses broken hosts. A Claude Code plugin layer (skills + slash commands) makes the CLI self-driving inside an agent. Both ship together.

**Not affiliated with Anthropic.** This project is an independent effort based on reverse-engineering the public Claude Desktop application.

## Why cowork-mdm

**Claude Desktop supports third-party Anthropic-compatible LLM providers.** Enterprises pick providers based on cost, data residency, compliance, and model preference — and Claude Desktop accepts all of them through `inferenceProvider=gateway` or dedicated Bedrock / Vertex / Foundry keys. That covers hyperscaler routes (AWS Bedrock, Google Vertex, Azure AI Foundry), self-hosted vLLM / SGLang behind an Anthropic-compatible shim, and third-party gateway services (DeepSeek, Zhipu GLM, MiniMax, Mistral's API, and similar). `cowork-mdm` lets IT deploy whichever choice at fleet scale.

**But the deployment config is non-trivial.** Gateway URL + auth scheme + managed MCP servers + egress allowlist + auto-update policy + telemetry policy + sandbox constraints — Claude Desktop reads 51 managed-preferences keys, of which Anthropic's public enterprise documentation covers only 8. End users can't self-serve this; IT has to push it at fleet scale via Jamf / Microsoft Intune / Kandji. `cowork-mdm` surfaces the full schema (extracted from the app's embedded zod schema, currently pinned to Claude.app 1.5354.0), generates correct MDM profiles, and runs per-host diagnostics so IT doesn't have to reverse-engineer the Electron bundle.

**MDM delivers the config layer; a Script payload delivers the rest.** LLM credentials, MCP servers, egress, telemetry, and sandbox policy all ride in the mobileconfig. Company skills, slash commands, and plugin-bundled MCPs live under `/Library/Application Support/Claude/org-plugins/` and are delivered via a companion Script payload that runs `cowork-mdm marketplace add`. This hybrid is necessary by design — the reverse-engineered evidence is documented in [docs/research/skills-plugins-mdm.md](docs/research/skills-plugins-mdm.md).

## Quick start

```bash
brew install krislavten/tap/cowork-mdm

# Happy path for an enterprise gateway deployment:
cowork-mdm profile show-template enterprise-cn-full --out overrides.yaml
$EDITOR overrides.yaml                           # fill REPLACE_* placeholders
cowork-mdm profile new --from overrides.yaml \
  --payload-identifier-prefix com.acme.it \
  --out company.mobileconfig
cowork-mdm profile lint company.mobileconfig    # pre-distribution gate
cowork-mdm profile validate company.mobileconfig
# Then push company.mobileconfig via your MDM — full recipe in the cookbook.
```

For Bedrock / Vertex / Foundry deployments, substitute the template name
— `bedrock-basic`, `vertex`, or `foundry` — and fill `{{ACCOUNT}}` /
region / model-ID placeholders instead. Same downstream pipeline.

## Enterprise deployment cookbook

**See [docs/deployment-cn.md](docs/deployment-cn.md)** — 8-section end-to-end recipe for gateway-mode deployments:

1. Prerequisites 2. Pick an LLM provider 3. Generate the profile 4. Validate & lint 5. Distribute via Jamf / Intune / Kandji (with Script payload for plugins) 6. Verify on an employee machine 7. Common failure modes 8. Updating later.

For Bedrock / Vertex / Foundry deployments the same CLI surface applies — start from [`specs/profile.md`](specs/profile.md) and use the `bedrock-basic` / `vertex` / `foundry` built-in templates.

## Command reference

Grouped by intent. Every subcommand accepts `--json` for machine-readable output.

**Author a profile**
```bash
cowork-mdm profile templates                           # list built-in templates
cowork-mdm profile show-template NAME [--out FILE]     # dump a template's YAML source
cowork-mdm profile new --from overrides.yaml --out out.mobileconfig
cowork-mdm profile lint out.mobileconfig               # flag REPLACE_* placeholders
cowork-mdm profile validate out.mobileconfig           # schema check
```
`--template` and `--from` are mutually exclusive. `lint` complements `validate`: validate is schema-only; lint catches unfilled scaffold placeholders.

**Inspect the schema**
```bash
cowork-mdm schema list                          # all 51 keys (name, type, scope, appMin)
cowork-mdm schema show inferenceProvider        # description, example, allowed values
cowork-mdm paths show [--os darwin|windows]     # paths cowork-mdm reads on each platform
```

**Apply & verify on a host**
```bash
cowork-mdm profile apply company.mobileconfig --dry-run    # preview, no writes
cowork-mdm profile status                                   # what's currently active
cowork-mdm doctor [--fix]                                   # diagnose broken installs
```

**Manage org plugins (macOS)**
```bash
cowork-mdm marketplace add https://github.com/<org>/claude-org-plugins
cowork-mdm marketplace update
cowork-mdm plugin list
cowork-mdm plugin prune
```

Spec and task breakdown: [`specs/`](specs/) + [`docs/execution/TASKS.md`](docs/execution/TASKS.md).

## Claude Code plugin

v0.3 also ships a Claude Code plugin layer so an agent can drive the CLI on the user's behalf — 5 skills + 4 slash commands covering profile authoring, deployment, plugin management, and diagnostics. Ships **no new logic**; requires the CLI on `PATH`. Full surface documented in [`specs/claude-plugin.md`](specs/claude-plugin.md).

Install in Claude Code:
```
/plugin marketplace add https://github.com/krislavten/cowork-mdm
/plugin install cowork-mdm@cowork-mdm
```

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
