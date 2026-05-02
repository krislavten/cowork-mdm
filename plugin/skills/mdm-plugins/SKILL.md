---
name: mdm-plugins
description: Managing Claude Desktop's org plugin library — cloning plugin marketplaces from git, updating them, linking individual plugins into the `org-plugins/` directory, and cleaning up dangling symlinks. Load when the user wants to install, update, or remove plugins/marketplaces for the Claude Desktop app (distinct from Claude Code plugins) — phrases like "install plugin", "add marketplace", "org-plugins", "plugin not showing up", "dangling symlink", "marketplace update", or asks about `/Library/Application Support/Claude/org-plugins/`.
---

# Claude Desktop org plugins

This skill covers plugins **for the Claude Desktop app** (the Electron
binary), not for Claude Code. The two systems share the "plugin" word but
live in different directories and have separate lifecycles.

- **Claude Desktop plugins** live under
  `/Library/Application Support/Claude/org-plugins/` and are visible to
  every user on that Mac.
- **Claude Code plugins** (what this very plugin is) install via
  `/plugin marketplace add` inside Claude Code.

This skill is about the first kind.

## The symlink model (you must understand this)

`org-plugins/` is a flat directory. Claude Desktop reads every top-level
entry and treats it as a plugin, **regardless of whether it's a real
directory or a symlink**.

`cowork-mdm` organizes that directory as follows:

```
/Library/Application Support/Claude/org-plugins/
├── claude-plugins-official/         ← real dir: `git clone` target
│   ├── .claude-plugin/marketplace.json
│   ├── plugins/<plugin-name>/…      ← actual plugin payloads
│   └── external_plugins/<name>/…
├── rush-plugin/                     ← second marketplace
│   └── plugins/<plugin-name>/…
│
│   ↓ top-level symlinks that cowork-mdm (re)generates:
├── agent-sdk-dev → claude-plugins-official/plugins/agent-sdk-dev
├── rush-ai      → rush-plugin/plugins/rush-ai
├── …
```

Every marketplace is one `git clone`. Top-level entries are **symlinks**
pointing into each marketplace's `plugins/` or `external_plugins/`
subdirectory. Claude Desktop sees flat names; updates are git pulls.

**If a top-level symlink dangles** (target moved/renamed), Claude Desktop
will log a warning and skip the plugin. `cowork-mdm plugin prune` removes
these.

## The four CLI commands you'll use

| Command | What it does |
| --- | --- |
| `cowork-mdm marketplace add URL [--name N]` | `git clone URL` into `org-plugins/N` (default: basename of URL) and auto-runs link-all. |
| `cowork-mdm marketplace update [NAME]` | `git pull` on one or all marketplaces, then rebuilds top-level symlinks. |
| `cowork-mdm marketplace list [--json]` | Shows marketplaces + HEAD SHA + plugin count. |
| `cowork-mdm marketplace remove NAME` | Removes the clone and every symlink that pointed into it. Prompts unless `--yes`. |

And at the individual-plugin level:

| Command | What it does |
| --- | --- |
| `cowork-mdm plugin list [--json]` | Lists every top-level entry, classified as `ok` / `dangling` / `real-dir`. |
| `cowork-mdm plugin show NAME` | Full detail: source marketplace, target, manifest, per-user enabled state. |
| `cowork-mdm plugin unlink NAME` | Removes a top-level symlink. Refuses to touch real directories. |
| `cowork-mdm plugin prune [--yes]` | Removes dangling symlinks. Dry-run by default. |

All commands that mutate `org-plugins/` require write access to
`/Library/Application Support/Claude/` — in practice, `sudo`. The plugin
never `sudo`s for you.

## Canonical workflow: install the official marketplace

```bash
cowork-mdm marketplace add https://github.com/anthropics/claude-plugins-public
# → clones to /Library/Application Support/Claude/org-plugins/claude-plugins-public
# → rebuilds top-level symlinks for every plugin in that repo

cowork-mdm marketplace list
# NAME                     HEAD        PLUGINS  LAST-PULL
# claude-plugins-public    abc1234     47       2026-05-01 10:22

cowork-mdm plugin list | head
```

Run it as your own user on macOS — most installs require **sudo** to write
into `/Library/Application Support/Claude/`. Hand the user the command
prefixed with `sudo`; don't run `sudo` yourself.

## Canonical workflow: daily refresh

```bash
cowork-mdm marketplace update           # pull every marketplace
cowork-mdm plugin prune                 # dry-run: lists dangling links
cowork-mdm plugin prune --yes           # commit the prune
```

The `/cowork-mdm:refresh-plugins` slash command wraps this.

## Canonical workflow: add a plugin from a new marketplace

```bash
cowork-mdm marketplace add https://github.com/example/claude-plugins
cowork-mdm plugin list --json | jq '.[] | select(.Source | contains("example"))'
```

`plugin list --json` uses PascalCase fields: `Name`, `Source` (e.g.
`marketplace:claude-plugins-official`), `TargetPath`, `IsSymlink`,
`Dangling`, `Manifest`. Parse with those exact keys.

No per-plugin install step exists — adding a marketplace auto-links every
plugin inside it. If you want to un-expose one plugin, `plugin unlink NAME`
removes just that top-level symlink while keeping the marketplace intact.

## Diagnosing "plugin doesn't show up"

```bash
cowork-mdm plugin list --json | jq '.[] | select(.Name=="THE_PLUGIN")'
```

Read the resulting entry:

- **`Dangling: true`** → the target path no longer exists. Most common
  cause: you `marketplace remove`d one marketplace that was providing this
  plugin. Fix: `plugin prune`.
- **`IsSymlink: false`** → it's a real directory at the top level, not a
  managed symlink. Usually from manually copying in a plugin. Either convert
  it to a marketplace-managed one or leave it alone and stop worrying.
- **not listed at all** → the marketplace that ships it isn't installed,
  or the marketplace's `.claude-plugin/marketplace.json` omits it. Check
  `marketplace list` and the source repo.

## Per-user enabled state

Claude Desktop stores each user's enabled/disabled plugin choices in the
user's session DB. `cowork-mdm plugin show NAME` reports this across all
users it can see — useful for confirming whether an install actually
reached the end user or just sits idle in `org-plugins/`.

## Gotchas

- **Don't delete a marketplace's real directory with `rm -rf`.** Use
  `cowork-mdm marketplace remove NAME` — it cleans up the symlinks too.
  Orphan symlinks in `org-plugins/` surface as dangling entries every
  Claude launch and spam the logs.
- **`org-plugins/` write requires elevated permissions on macOS.** The CLI
  fails loud if it can't write. Prefix with `sudo` — the user runs it.
- **Marketplaces are git repos, not packages.** There's no version pinning;
  whatever `main` (or the branch you cloned) has at pull time is what
  ships. If you need a pinned state, clone manually into `org-plugins/`
  and accept that `cowork-mdm marketplace update` will push it forward.
