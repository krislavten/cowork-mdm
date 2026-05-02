# Can MDM deliver skills / plugins to Claude Desktop?

Research artifact for [issue #31](https://github.com/krislavten/cowork-mdm/issues/31).

Extraction target: `Claude.app` version `1.5354.0`, macOS, `Contents/Resources/app.asar → .vite/build/index.js` (15.1 MB minified).

## TL;DR

**No — based on Claude Desktop 1.5354.0's known managed-preferences and bootstrap channels, MDM cannot directly deliver skills or plugins.** The 51-key managed-preferences schema contains zero plugin/skill/marketplace keys, and the `bootstrapUrl` overlay pathway is gated by a compile-time allowlist (`CSt = Kf.filter(e => qC(e).scopes.includes("3p-bootstrap"))`) that only admits 17 LLM/OTLP keys. The `/Library/Application Support/Claude/org-plugins/` directory (scanned at launch) is hardcoded and cannot be relocated via schema — but it *can* be populated by an IT-run post-install script that wraps `cowork-mdm marketplace add`. This is the enterprise-delivery path we verified; it is not pure-MDM — it requires a script channel. An exhaustive bundle audit for other non-schema plugin-delivery paths was **not** done (see Limitations below) — the conclusion is bounded to the channels explicitly investigated.

## Evidence table

| # | Hypothesis | Method | Result | Citation |
| - | --- | --- | --- | --- |
| 1 | Schema exposes a `managedPlugins` / `managedSkills` / `orgPluginsMarketplace` key | `grep` the full 51-key schema dump via `cowork-mdm schema list --json` + scan the embedded zod schema in the bundle for name fragments `plugin`, `skill`, `market` | **None found.** All 51 keys belong to inference / MCP / telemetry / bootstrap / update / sandbox. No key accepts a plugin manifest, marketplace URL, or skill bundle path. | `internal/schema/schema.json` (all keys); `.vite/build/index.js` FJ=me({…}) block grep = 0 matches for `managedPlugin*` / `managedSkill*` |
| 2 | `bootstrapUrl` remote config overlay can carry arbitrary keys (e.g. `{"managedPlugins":[…]}`) | Read the bootstrap fetcher (`async function far(e,A,t)`) and the allowlist filter (`eji(o, trustedOrigin)`). Identify what `PQA` and `CSt` do to the response JSON. | **Strictly filtered.** `CSt = Kf.filter(e => qC(e).scopes.includes("3p-bootstrap"))` — only keys whose schema `scopes` array contains `"3p-bootstrap"` are kept; unknown keys are silently dropped with log line `"bootstrap response contained no recognized keys; ignoring"`. The 3p-bootstrap allowlist has **17 keys**, all LLM / OTLP: `inferenceProvider`, `inference{Gateway,Vertex,Bedrock,Foundry}*`, `inferenceModels`, `otlp*`. No plugin/skill/marketplace keys. | `.vite/build/index.js`: `CSt=Kf.filter(e=>qC(e).scopes.includes("3p-bootstrap"))`; bootstrap fetcher body (`far`) and response parser (`PQA`) |
| 3 | `org-plugins/` directory path is relocatable via a schema key (e.g. to a network mount MDM pushes) | Locate the path resolver in the bundle. | **Hardcoded.** `function NF(){ switch(process.platform){case"darwin":return"/Library/Application Support/Claude/org-plugins";case"win32":return rA.join("C:\\Program Files","Claude","org-plugins");default:return null}}`. No env var, no schema key. Directory is scanned at launch (`readdir` via `Xvr(e)`), versioned by `.org-plugin-version` file, prefix-tagged `"[custom-3p:org-plugins]"`. | `.vite/build/index.js`: function `NF()`, const `Ixt=".org-plugin-version"`, const `zM="[custom-3p:org-plugins]"` |
| 4 | Claude Desktop honors Claude Code CLI's managed-settings surface (`extraKnownMarketplaces`, `strictKnownMarketplaces`, `CLAUDE_CODE_PLUGIN_SEED_DIR`) — they are documented as working for the CLI | `grep` the Desktop bundle for each of those literal names. | **Desktop reads the same `managed-settings.json` file** (`/Library/Application Support/ClaudeCode/managed-settings.json` on macOS — `function oLn()` returns that path), but it **only reads `enabledPlugins`** out of it. **No match** for `extraKnownMarketplaces`, `strictKnownMarketplaces`, `blockedMarketplaces`, or `CLAUDE_CODE_PLUGIN_SEED_DIR`. Those are Claude Code CLI features; Desktop's plugin subsystem is separate. | `.vite/build/index.js`: `function oLn()` path resolver; `rg "enabledPlugins|extraKnownMarketplaces|strictKnownMarketplaces|CLAUDE_CODE_PLUGIN_SEED"` = only `enabledPlugins` present |
| 5 | Anthropic's public enterprise-deployment docs describe an MDM path for plugins/skills | Read <https://support.claude.com/en/articles/10106014-deploying-claude-s-desktop-app-for-enterprise-use> and <https://code.claude.com/docs/en/plugin-marketplaces>. | **Desktop enterprise doc**: 404 / no coverage. **Claude Code CLI doc**: describes `CLAUDE_CODE_PLUGIN_SEED_DIR` + `extraKnownMarketplaces` — **but for Claude Code, not Claude Desktop**. No public Anthropic doc covers plugin/skill delivery to Claude Desktop via any channel. | Web fetch transcripts (tool-results/2026-05-02) |

## Current state

The concrete enterprise-delivery story for **Claude Desktop** as of `Claude.app 1.5354.0`:

| Asset | Delivery channel today | MDM-native? |
| --- | --- | --- |
| Inference provider config (Bedrock / Vertex / gateway / Foundry) | `.mobileconfig` via `inferenceProvider` + friends | ✅ yes (17-key bootstrap scope, or full-fat 3p scope on the plist) |
| Standalone MCP servers | `.mobileconfig` via `managedMcpServers` (scope `3p`) | ✅ yes — but **not via bootstrap**; has to be on the mobileconfig. The `3p-bootstrap` scope does NOT include `managedMcpServers`, so an MCP list cannot be rotated via the bootstrap endpoint. Note: this only covers **standalone** MCP server configs; MCP servers **bundled inside a plugin** (declared in the plugin's `plugin.json`) ride along with the plugin and are delivered through the plugin-delivery path below, not this key. |
| Remote-updateable LLM + OTLP config | `bootstrapUrl` → HTTPS endpoint that returns JSON overlay of 3p-bootstrap-scoped keys | ✅ yes, but only the 17 allow-listed keys |
| Auto-update policy, telemetry opt-out, egress allowlist, sandbox folders | `.mobileconfig` via their respective schema keys | ✅ yes |
| **Plugins + skills + slash commands + hooks** | Local filesystem at `/Library/Application Support/Claude/org-plugins/<plugin>/` — populated either by a user running `/plugin marketplace add` inside Claude Desktop, or by `cowork-mdm marketplace add <url>` (clones a git marketplace repo and symlinks discovered plugins into place) | ❌ **no MDM surface**. IT must wrap delivery in a post-install script that calls `cowork-mdm marketplace add`; the act of populating that directory is not itself an MDM operation |
| Which installed plugins are enabled | `enabledPlugins` field in `/Library/Application Support/ClaudeCode/managed-settings.json` (shared with Claude Code CLI) | ⚠️ partial — **enable/disable only, does not register new marketplaces**. MDM can push a managed-settings.json that locks the enabled set, but the actual plugin files still have to be present in `org-plugins/` first |

**The gap**: for "company skills, slash commands, and plugin-bundled MCPs", `cowork-mdm`'s existing `marketplace add` is the right mechanism, but its invocation is out-of-band w.r.t. MDM. IT gets an all-or-nothing choice:

1. **Pure MDM** — LLM + standalone MCP + telemetry + sandbox. No plugins / skills / plugin-bundled MCPs.
2. **MDM + post-install script** — MDM pushes the mobileconfig AND the MDM system also runs `cowork-mdm marketplace add <org-plugins-repo>` as a scripted payload on the same host. Jamf calls this a "Script" payload; Intune has "Shell scripts"; Kandji has "Custom Script". Of the channels investigated here, this is the way to bundle plugin delivery with MDM delivery.

### Limitations of this investigation

This audit covered: the zod schema (all 51 keys), the `bootstrapUrl` fetch-and-filter pipeline (`far` / `PQA` / `CSt`), the `org-plugins/` path resolver (`NF`), the `managed-settings.json` consumer path, and Anthropic's public enterprise + Claude Code plugin-marketplace docs. An exhaustive bundle grep for every possible plugin-delivery code path was **not** performed. If such a path exists (e.g. an unused-but-present `managedPlugins*` reader in a code path we didn't find), the conclusion would need revisiting. The verdict is bounded to "the channels documented or reverse-engineered above."

## Gap analysis — what would unblock pure-MDM delivery?

For Anthropic to make MDM-native plugin/skill delivery possible, the Claude Desktop schema would need new keys. Proposed shapes (concrete enough to fit the existing zod schema pattern):

| Proposed key | Type | Scope | Intent |
| --- | --- | --- | --- |
| `managedPluginMarketplaces` | `stringArray` | `3p,3p-bootstrap` | JSON array of git-clone-able URLs. On launch, Claude Desktop clones each into `org-plugins/` (equivalent to the user running `/plugin marketplace add <url>` for each entry). Read-only for users once set via MDM. |
| `managedPluginsLockfile` | `jsonString` | `3p,3p-bootstrap` | A pinned list `[{name, marketplaceName, version}]` — Claude Desktop installs exactly these plugins from the above marketplaces, no others. Analogous to a `package-lock.json`. |
| `managedSkillsBundleUrl` | `url` | `3p,3p-bootstrap` | HTTPS endpoint that returns a zip/tarball of a `skills/` tree. Replaces or supplements local skills. Optional — may be subsumed by `managedPluginMarketplaces` if Anthropic insists all skills ship inside plugins. |
| `blockedPluginMarketplaces` | `stringArray` | `3p` | Symmetric with `strictKnownMarketplaces` on the CLI — hard block list of marketplace URLs. |

The precedent is already there: Claude Code CLI ships `extraKnownMarketplaces` / `strictKnownMarketplaces` / `CLAUDE_CODE_PLUGIN_SEED_DIR`. Porting equivalents to Claude Desktop's zod schema + bootstrap allowlist is not a research problem, it is a product-scope decision.

## Recommendation for cowork-mdm

### Short term (what we ship now)

1. **Do not invent MDM-native plugin keys in `cowork-mdm`.** The CLI has a firm rule: "no schema keys Claude Desktop doesn't read." Inventing `managedPlugins` in our schema would break that rule and mislead users.
2. **Document the hybrid path** in the issue #30 cookbook Part 3 (see verbatim text below). The honest story: profiles + mobileconfig for LLM/MCP/telemetry; `cowork-mdm marketplace add` wrapped in a post-install script for plugins/skills.
3. **Extend `cowork-mdm` with a `marketplace add` scripted mode** that's friendly to being dropped into a Jamf/Intune/Kandji script payload — idempotent, non-interactive, exit-coded, JSON-output-friendly. Most of this already exists; confirm in a follow-up issue if anything is missing.

### Long term (if Anthropic moves)

Track the Claude Desktop zod schema for new keys on each release. The `internal/schema/extract/` tool already regenerates `schema.json` from `Claude.app`; if a new major version adds a `managedPlugins*` key, `cowork-mdm` picks it up automatically. When that happens, open a follow-up issue `task-mdm-plugins` to wire a `cowork-mdm plugin deploy --mdm` path.

### Part 3 text to land in issue #30's cookbook

Copy-paste-able into `docs/deployment-cn.md` Part 3:

> ### Delivering skills, slash commands, and plugin-bundled MCPs
>
> Standalone MCP servers go in the mobileconfig via `managedMcpServers`. But **skills, slash commands, hooks, and plugin-bundled MCPs do not ship inside the mobileconfig.** Claude Desktop reads those from its local `/Library/Application Support/Claude/org-plugins/` directory. That directory is populated in one of three ways:
>
> 1. **End user runs `/plugin marketplace add <url>`** inside Claude Desktop.
> 2. **IT pre-populates via `cowork-mdm marketplace add <url>`** on first setup. This clones the git repo into `org-plugins/` and symlinks each plugin it finds.
> 3. **MDM runs `cowork-mdm marketplace add <url>` as a scripted payload** on the same cadence as the mobileconfig push. This is what we recommend for enterprise fleets. Jamf: "Script" payload. Intune: "Shell scripts for macOS". Kandji: "Custom Script".
>
> The mobileconfig carries **LLM config** (`inferenceProvider`, gateway URL, model list) and **standalone MCP config** (`managedMcpServers`) — not skills, not plugins, and not plugin-bundled MCPs. `bootstrapUrl` cannot deliver skills or plugins either: its JSON response is filtered by a compile-time allowlist that only admits LLM and OTLP keys.
>
> If your org has its plugin marketplace ready (a git repo with the standard `.claude-plugin/marketplace.json` layout), ship it as part of the same deployment wave:
>
> ```bash
> cowork-mdm marketplace add https://github.com/<your-org>/claude-org-plugins
> cowork-mdm marketplace update      # later, to refresh
> cowork-mdm plugin list             # verify what resolved
> ```

## Appendix: draft upstream feature request

If we decide to ask Anthropic for MDM-native plugin delivery, the following is ready to submit as a GitHub issue or support ticket. **Not yet submitted** — maintainer decision.

> **Title**: Add managed-preferences keys for plugin marketplace + enabled-plugin lockfile to Claude Desktop
>
> **Summary**: Claude Desktop's managed-preferences schema (the zod schema embedded in the Electron bundle, currently 51 keys) has no way for IT to deliver or lock down plugins, skills, slash commands, or plugin-scoped MCP servers. The `bootstrapUrl` overlay mechanism's key allowlist (`CSt = Kf.filter(e => qC(e).scopes.includes("3p-bootstrap"))`) only admits 17 LLM/OTLP keys.
>
> **Use case**: enterprise IT deploying Claude Desktop to hundreds of engineer workstations wants to ship company-approved skills and slash commands on the same cadence as the LLM configuration. Today this requires an out-of-band scripted post-install step that wraps filesystem mutation of `/Library/Application Support/Claude/org-plugins/` — which is fragile and easy to skip.
>
> **Proposed keys** (aligned with precedent set by Claude Code CLI's `extraKnownMarketplaces` / `strictKnownMarketplaces` / `CLAUDE_CODE_PLUGIN_SEED_DIR`, adapted to Desktop's schema shape):
>
> - `managedPluginMarketplaces: stringArray`, scope `3p,3p-bootstrap` — JSON array of git URLs to auto-clone on launch
> - `managedPluginsLockfile: jsonString`, scope `3p,3p-bootstrap` — pinned `{name, marketplaceName, version}[]` list
> - `blockedPluginMarketplaces: stringArray`, scope `3p` — hard blocklist
>
> **What this unlocks**: MDM-native "full enterprise onboarding" with one configuration channel — no scripted post-install payload required, no user interaction needed, no sudo required for the plugin directory (app-managed).
