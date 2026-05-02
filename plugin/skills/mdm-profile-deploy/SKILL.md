---
name: mdm-profile-deploy
description: Deploying a Claude Desktop MDM profile to real user machines — choosing between MDM channel push (Jamf/Intune/Kandji/Mosyle) vs local apply, verifying status on a target host, and debugging profiles that don't take effect. Load when the user wants to push, install, activate, or verify an already-authored profile — phrases like "apply this mobileconfig", "push via Jamf/Intune", "profile status", "why isn't my config taking effect", "Claude Desktop isn't reading my settings", or anything about `/Library/Managed Preferences/`.
---

# MDM profile deploy

This skill covers **pushing an already-authored profile to hosts** and
**verifying it took effect**. For authoring, see `mdm-profile-authoring`.

## The single most important rule (read this first)

> **Direct writes to `/Library/Managed Preferences/` are CLOBBERED by
> `managedappconfigd` on the next MDM sync.**

macOS's `managedappconfigd` owns that directory. It rewrites it every time
the system checks in with its MDM profile source. If you `sudo cp` a plist
there:

- It works **for a few hours** (or until next sync, reboot, or network
  bounce).
- Then macOS silently overwrites or deletes it.
- The user reports "Claude forgot its config" and you can't reproduce it
  because by then it's gone.

**Production deployments must go through an MDM channel.** Period. The
correct artifact to hand the IT admin is a signed `.mobileconfig`, not a
plist and not a shell command.

Local `apply` exists only for **developer loops on a dev machine** — and
even then, `--dry-run` is almost always what you want.

## Deployment paths (pick one)

### Path A — MDM channel (the production path)

1. Generate a `.mobileconfig` with `cowork-mdm profile new … --out profile.mobileconfig`.
2. Validate: `cowork-mdm profile validate profile.mobileconfig`.
3. Hand the signed / unsigned `.mobileconfig` to the IT admin for upload:
   - **Jamf Pro** — Computers → Configuration Profiles → Upload → scope to
     a smart group / target machines.
   - **Intune** — Devices → Configuration profiles → Create → macOS →
     Templates → Custom → upload.
   - **Kandji / Mosyle / Addigy** — their "Custom Profile" workflow.
4. The MDM server pushes the profile; `managedappconfigd` writes the plist
   into `/Library/Managed Preferences/…` and **keeps it there**.
5. Verify on a target host with `cowork-mdm profile status` (next section).

### Path B — Local apply (dev loop only, not production)

`cowork-mdm profile apply` writes the plist directly. Use only to iterate
on a profile on your own machine:

```bash
sudo cowork-mdm profile apply profile.mobileconfig --dry-run
# inspect the preview, then:
sudo cowork-mdm profile apply profile.mobileconfig
```

Two safety rails:

- **Always run `--dry-run` first** and confirm the preview matches intent.
- **Never invoke `sudo` on the user's behalf** — hand them the command,
  let them run it. This plugin has no business elevating.

If the user asks you to `sudo apply` in production, push back with the rule
above and offer to hand them a `.mobileconfig` instead.

## Verifying status

`cowork-mdm profile status` reads the active plist / registry key and
reports what's currently live. The JSON form is the source of truth for
the field contract; the human form is a rendering for eyeballs and may
omit fields that are absent / null.

JSON output (parse this to reason programmatically):

```bash
cowork-mdm profile status --json
```

Shape:

```
{
  "platform":    "darwin" | "windows",
  "targetPath":  string,                      // plist path or registry key
  "present":     bool,
  "profile": {
    "name":   string,                         // "" when decoded from an on-disk plist
    "values": { "<key>": <value>, ... }       // map, NOT array — jq with .profile.values["KEY"]
  },
  "unknownKeys": [ {"key":"…","raw":"…"}, ... ] | null,
  "parseError":  string                       // "" when parse succeeded
}
```

Notes that bite parsers:
- `profile.values` is a **JSON object** (map keyed by preference name),
  not an array. Access with `jq '.profile.values.inferenceProvider'`.
- `unknownKeys` is `null` when there are none, not `[]`. Guard with
  `jq '.unknownKeys // [] | length'`.
- The human-output table **does not** include an `unknown: 0` line — only
  `unknownKeys` in JSON carries that data.

### "I pushed but status says not present"

- The profile payload type must be `com.anthropic.claudefordesktop` (the
  app's bundle identifier). Intune/Jamf upload flows sometimes let you pick
  wrong payload identifiers.
- After MDM push, macOS needs a check-in. `sudo profiles renew -type
  configuration` forces it.
- `cowork-mdm doctor` includes `plist.exists` and `plist.schema` checks
  — switch to `mdm-doctor` skill for the full decision tree.

### "Profile is present but Claude Desktop ignores it"

- Claude Desktop only reads MDM config at **launch**. Fully quit
  (`cmd-Q`, not just close-window) and relaunch.
- Verify the `appMin` of each key with `cowork-mdm schema show KEY`. If the
  user's Claude Desktop is older than the `appMin`, that key is silently
  ignored.
- Check `unknownKeys`: `cowork-mdm profile status --json | jq '.unknownKeys
  // []'`. A non-empty list means your plist has a typo or the schema
  drifted; `null` or `[]` is clean.

## System-scope vs user-scope plists

Claude Desktop reads both:

- `/Library/Managed Preferences/com.anthropic.claudefordesktop.plist`
  (system — applies to all users)
- `/Library/Managed Preferences/<username>/com.anthropic.claudefordesktop.plist`
  (per-user — overrides system for that user)

Most deployments use the system plist. The per-user variant is useful when
different groups need different `inferenceProfile` values. `cowork-mdm
profile status` reports whichever it finds; with `--json` both surface if
both exist.

## Pre-flight checklist before any push

```bash
cowork-mdm profile validate profile.mobileconfig           # schema clean; exit 0 on success
cowork-mdm profile apply profile.mobileconfig --dry-run    # preview exact bytes
cowork-mdm profile status --json                           # what's already live on this host
```

If any of these three fail, do not ship.

## Gotchas specific to deploy

- **Rotating MCP tokens inside `managedMcpServers`** — when an MDM profile
  is replaced by a new version, the old plist is rewritten. If you depend
  on old tokens being invalidated, also rotate them at the provider side;
  the plist rewrite alone doesn't clear app caches.
- **Signed vs unsigned mobileconfigs** — `cowork-mdm profile new` emits
  **unsigned** `.mobileconfig`. Most MDMs re-sign during upload. If you
  need to test locally by double-click, expect a "Profile unsigned" warning
  — that's expected, not a bug.
- **`disableDeploymentModeChooser: true` gates hard on the provider being
  fully configured.** If you ship this flag but `inferenceProvider` /
  `inferenceBedrockRegion` / `inferenceBedrockProfile` are missing, users
  hit a dead screen with no way to recover. Keep the chooser enabled until
  you're confident the profile is complete, then flip the flag.
