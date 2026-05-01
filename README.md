# cowork-mdm

> CLI toolkit for deploying Claude Desktop in the enterprise: MDM config profiles, plugin marketplace management, and per-host diagnostics.

**Status**: **v0.1 — schema + path reference.** v0.2 adds profile generation, marketplace management, and diagnostics (in progress — see [docs/execution/TASKS.md](docs/execution/TASKS.md)).

**Not affiliated with Anthropic.** This project is an independent effort based on reverse-engineering the public Claude Desktop application.

## Why

Anthropic's public enterprise documentation covers **8 of the 51** MDM keys that `Claude.app` actually reads. The remaining keys — `inferenceProvider`, `inferenceBedrockRegion`, `managedMcpServers`, `coworkEgressAllowedHosts`, `bootstrapUrl`, and more — are defined in the app's embedded zod schema (`FJ = me({...})`) but undocumented publicly.

Deploying Claude Desktop in 3rd-party inference mode (Bedrock, Vertex, LLM gateway, Azure Foundry) relies heavily on these undocumented keys. `cowork-mdm` surfaces the schema, generates correct config profiles (`.mobileconfig` / `.reg` / Jamf / Intune formats), manages the org plugin marketplace, and runs per-host diagnostics — so IT admins don't have to reverse-engineer the Electron bundle themselves.

## Quick start

```bash
# macOS (Homebrew)
brew tap krislavten/tap
brew install cowork-mdm

# Or download a binary from the Releases page:
# https://github.com/krislavten/cowork-mdm/releases
```

## Commands

### Shipped in v0.1

```bash
cowork-mdm schema list                     # all 51 keys (name, type, scope, appMin)
cowork-mdm schema show inferenceProvider   # details: description, example, allowed values

cowork-mdm paths show                      # host paths cowork-mdm reads
cowork-mdm paths show --os windows         # simulate a different platform

cowork-mdm --version
```

Both subcommands accept `--json` for machine-readable output.

### Planned for v0.2

```bash
# Profile generation
cowork-mdm profile new --template bedrock-mcp --out my.mobileconfig
cowork-mdm profile validate my.mobileconfig
cowork-mdm profile export my.mobileconfig --format reg

# Marketplace + plugin management (macOS)
cowork-mdm marketplace add https://github.com/anthropics/claude-plugins-official
cowork-mdm marketplace update
cowork-mdm plugin list / prune

# Diagnostics
cowork-mdm doctor
cowork-mdm doctor --fix
```

Spec and task breakdown: [`specs/`](specs/) + [`docs/execution/TASKS.md`](docs/execution/TASKS.md).

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
