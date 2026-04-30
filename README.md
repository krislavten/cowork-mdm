# cowork-mdm

> CLI toolkit for deploying Claude Desktop in the enterprise: MDM config profiles, plugin marketplace management, and per-host diagnostics.

**Status**: pre-release / v0.2 in development. See [docs/execution/TASKS.md](docs/execution/TASKS.md) for current progress.

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

### Schema reference

```bash
cowork-mdm schema list                     # all 51 keys (name, type, scope, appMin)
cowork-mdm schema show inferenceProvider   # details: description, example, legacyAlias
```

### Profile generation

```bash
cowork-mdm profile new                                    # interactive wizard (TUI)
cowork-mdm profile new --template bedrock-mcp             # non-interactive from template
cowork-mdm profile validate my.mobileconfig               # check against schema
cowork-mdm profile export my.mobileconfig --format reg    # convert macOS → Windows
cowork-mdm profile apply my.mobileconfig                  # local dev: sudo cp to /Library/Managed Preferences/
cowork-mdm profile status                                 # what's currently applied
```

### Marketplace management (macOS)

```bash
cowork-mdm marketplace add https://github.com/anthropics/claude-plugins-official
cowork-mdm marketplace update               # git pull all marketplaces + rebuild links
cowork-mdm plugin list                      # per-plugin source + link state
cowork-mdm plugin prune                     # remove dangling symlinks
```

### Diagnostics

```bash
cowork-mdm doctor                           # check plist, pkg, LaunchAgent, AWS creds, app state
cowork-mdm doctor --fix                     # auto-repair what's auto-repairable
```

## Supported platforms

| | macOS | Windows | Linux |
|---|---|---|---|
| `schema *` | ✅ | ✅ | ✅ |
| `profile *` | ✅ `.mobileconfig` | ✅ `.reg` | build-only |
| `marketplace *`, `plugin *` | ✅ | ❌ v0.2 | ❌ |
| `doctor` | ✅ | ✅ registry-based | ❌ |

## Contributing

See [AGENTS.md](AGENTS.md) for development conventions. Issues and PRs welcome.

## License

MIT — see [LICENSE](LICENSE).
