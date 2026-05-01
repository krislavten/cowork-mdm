---
allowed-tools: Bash(cowork-mdm:*)
description: Update all Claude Desktop plugin marketplaces and clean dangling symlinks
---

## Context

- cowork-mdm version: !`cowork-mdm --version 2>&1`
- Current marketplaces: !`cowork-mdm marketplace list --json 2>&1`
- Current plugins: !`cowork-mdm plugin list --json 2>&1`
- Dangling symlinks (dry-run prune): !`cowork-mdm plugin prune 2>&1`

## Your task

Drive the daily-refresh workflow for Claude Desktop plugin marketplaces.
See the `mdm-plugins` skill for the underlying model.

### Steps

1. **Report current state** from the context above — how many
   marketplaces, how many plugins, how many dangling links.

2. **Update all marketplaces** (needs sudo — ask the user to run it;
   do NOT sudo yourself):

   ```bash
   sudo cowork-mdm marketplace update
   ```

   After the user runs it, re-read `cowork-mdm marketplace list --json`
   and report any change in HEAD SHAs or plugin counts.

3. **Re-check plugins** with `cowork-mdm plugin list --json`. Compare
   against the pre-update snapshot from context: note added, removed,
   newly-dangling.

4. **Prune dangling** only if any appeared. Ask the user to run:

   ```bash
   sudo cowork-mdm plugin prune --yes
   ```

   Do not run `sudo` yourself. Explain that prune removes symlinks only,
   never real directories.

5. **Final summary** — total marketplaces, total live plugins, HEAD SHAs.

### Rules

- Never run `sudo`. Hand the command to the user.
- Never `rm -rf` anything under `/Library/Application Support/Claude/`.
  The only way to remove a marketplace is `cowork-mdm marketplace remove`.
- If a marketplace has a non-fast-forward git state (the `update` output
  reports a conflict), stop and ask the user before doing anything
  destructive — someone may have hand-edited files in the clone.
