---
allowed-tools: Bash(cowork-mdm:*)
description: Validate and preview an MDM profile before MDM-channel deployment
argument-hint: PATH_TO_MOBILECONFIG
---

## Context

- Profile under review: `$1`
- cowork-mdm version: !`cowork-mdm --version 2>&1`
- Validation: !`cowork-mdm profile validate "$1" 2>&1`
- Dry-run preview (first 60 lines): !`cowork-mdm profile apply "$1" --dry-run 2>&1 | head -60`
- Current managed plist status on this host: !`cowork-mdm profile status --json 2>&1`

## Your task

Given a `.mobileconfig` (or `.plist`) at `$1`, walk the user through a
pre-deployment review. **This command never writes to disk.** It validates,
previews, and hands off.

### Steps

1. **Check validation** — if the `cowork-mdm profile validate` output
   above reports errors, stop and surface them to the user. Do not
   proceed to deployment guidance until validation is clean.

2. **Summarize the profile** — read the dry-run preview context above and
   tell the user which keys will be set, which provider is configured,
   and whether any high-impact flags (`disableDeploymentModeChooser`,
   broad `coworkEgressAllowedHosts`) are turned on.

3. **Diff against current host state** — the `cowork-mdm profile status
   --json` context shows what's currently active on this Mac. Compare
   the two key sets and call out:
   - keys being **added** by this profile
   - keys being **changed** (value differs)
   - keys being **removed** (present today, absent in the new profile)

4. **Hand off for MDM deployment** — the right path is:
   - For production: upload to Jamf / Intune / Kandji / Mosyle /
     Addigy as a custom configuration profile with the payload type
     `com.anthropic.claudefordesktop`. Do NOT try to do this from the
     CLI — that's the admin console's job.
   - For a local developer test only: tell the user they can run
     `sudo cowork-mdm profile apply "$1"` themselves, but warn them
     that macOS's `managedappconfigd` will clobber the plist on the
     next MDM sync, so this is not a production path.

### Rules

- **Never** run `sudo cowork-mdm profile apply`. Never write into
  `/Library/Managed Preferences/` directly. This plugin does not elevate.
- If the user asks you to deploy in production without going through
  their MDM, refuse and explain the `managedappconfigd` clobber from
  the `mdm-profile-deploy` skill.
- If validation failed, do not show the dry-run preview to the user as
  "what will happen" — the profile is invalid and the preview is
  misleading.
