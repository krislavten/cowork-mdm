---
allowed-tools: Bash(cowork-mdm:*)
description: Diagnose the local Claude Desktop install using cowork-mdm doctor
---

## Context

- cowork-mdm version: !`cowork-mdm --version 2>&1`
- Host paths: !`cowork-mdm paths show --json 2>&1`
- Doctor report: !`cowork-mdm doctor --json 2>&1`

## Your task

Parse the JSON doctor report from context and help the user understand
each finding. Follow the `mdm-doctor` skill's decision tree for
interpretation.

### Steps

1. **Parse and group** the JSON findings by status:
   - `error` — needs attention (blockers)
   - `warning` — informational / non-blocking
   - `ok` — mention count, don't list each
   - `skipped` — only mention if the user is on an unsupported platform

2. **For each error/warning**, in order:
   - Restate the finding in plain language.
   - Suggest the most likely cause (from the `mdm-doctor` skill's
     decision tree).
   - Propose a specific next command, not `--fix` blindly.

3. **Decide whether to recommend `--fix`** — only if ALL remaining non-OK
   findings have `fixAvailable: true` AND the fixes are low-risk
   (creating the `org-plugins/` directory, pruning dangling symlinks).
   Do not suggest `--fix` for anything that would touch
   `/Library/Managed Preferences/` (the doctor doesn't attempt those
   anyway, but surface the distinction to the user).

4. **Offer hand-offs** when a finding is better handled by another skill:
   - Plist issues → `mdm-profile-deploy`
   - Plugin/symlink issues → `mdm-plugins`
   - Profile authoring questions → `mdm-profile-authoring`

### Rules

- Do **not** run `cowork-mdm doctor --fix` without asking the user first.
- Do **not** `sudo` anything.
- If the doctor context above shows a CLI error (e.g. CLI not on PATH),
  stop and ask the user to install `cowork-mdm` — the rest of the
  diagnosis is meaningless without it.
