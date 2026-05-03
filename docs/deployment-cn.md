# Claude Desktop enterprise deployment — gateway-mode cookbook

End-to-end recipe for taking Claude Desktop from "fresh Mac" to "employee
logged in, talking to the company-approved LLM provider with company MCP
servers and company skills / slash commands". Target audience: enterprise
IT / ops running Jamf Pro, Microsoft Intune, or Kandji for macOS fleets.

Covers the `inferenceProvider=gateway` path: third-party Anthropic-
compatible services (DeepSeek, Zhipu GLM, MiniMax, Mistral, …) and
self-hosted vLLM / SGLang behind an Anthropic-compatible shim. For
Bedrock / Vertex / Foundry deployments use the `bedrock-basic` /
`vertex` / `foundry` built-in templates instead; the distribution
mechanics (Sections 5–8) are identical.

Scope: macOS. Windows is not yet supported end-to-end (tracked in
[issue #9](https://github.com/krislavten/cowork-mdm/issues/9)).

> **Key fact up front**: a `.mobileconfig` (the MDM-native channel)
> delivers LLM + standalone MCP + telemetry + sandbox policy only. It
> **cannot** deliver skills, slash commands, hooks, or plugin-bundled
> MCPs — see [`docs/research/skills-plugins-mdm.md`](research/skills-plugins-mdm.md)
> for the reverse-engineered evidence. Full onboarding therefore
> combines an MDM mobileconfig push with a Script payload that runs
> `cowork-mdm marketplace add <org-plugins-repo>`. Section 5 shows both.

## 1. Prerequisites

Before starting, gather:

- **Vendor API key.** Created on the provider's platform console (e.g.
  DeepSeek, Zhipu GLM, MiniMax, Mistral, or your self-hosted gateway's
  admin console). You'll inject this via your MDM's secret-variable
  mechanism, not by hand-pasting into a YAML file.
- **Admin access to your MDM system** (Jamf Pro / Intune / Kandji) —
  specifically the permission to push a Custom Settings Payload + a
  Shell Script payload, both scoped to the target device group.
- **List of your internal MCP server endpoints.** Typically `https://
  mcp.<your-org>.internal/<service>` entries. Each needs `{name, url,
  transport}` and optionally `toolPolicy` (see
  `cowork-mdm schema show managedMcpServers`).
- **Optional but recommended — a plugin marketplace repo.** A git repo
  following [Anthropic's marketplace layout](https://code.claude.com/docs/en/plugin-marketplaces)
  (`.claude-plugin/marketplace.json` + `plugins/<name>/...`). This is
  how skills / slash commands / hooks reach employee machines. Without
  one, the Script payload step in Section 5 is skipped and you ship LLM
  config only.
- **A macOS 13+ test machine** you can enroll in your MDM's staging group
  to verify the deployment end-to-end before rolling to the fleet.
- **`cowork-mdm` CLI on the admin workstation** — install via
  `brew install krislavten/tap/cowork-mdm`. You need it to generate,
  validate, and preview the profile before pushing.
- **`cowork-mdm` CLI on employee Macs** — every target Mac needs the
  binary present so the Wave 2 Script payload (Section 5) can run
  `marketplace add`. The recommended enterprise path is the `.pkg`
  installer from each GitHub Release
  (`cowork-mdm_<version>_darwin_<arch>.pkg`), pushed via your MDM's
  package-deployment mechanism *before* Wave 2 runs. The pkg lands
  the binary at `/opt/cowork-mdm/bin/cowork-mdm` and symlinks it at
  `/usr/local/bin/cowork-mdm`. **Before pushing, confirm the pkg is
  signed** — Jamf Pro / Kandji / Intune all require a Developer ID
  Installer signature (or an internally trusted cert) on any `.pkg`
  they deploy. Release-page pkgs are unsigned until the repo's Apple
  signing secrets are configured; if your copy is unsigned, either
  re-sign with your org's internal Developer ID / MDM cert before
  pushing, or fall back to brew on a test machine. The Wave-2
  resolver below finds both pkg and brew layouts.

## 2. Pick your LLM provider

Pre-baked templates cover several gateway providers with Anthropic-
compatible endpoints. Pick one and note its values — you'll use them in
Section 3. If your provider isn't listed, start from the generic
`gateway` template and supply the base URL + auth scheme yourself.

| Vendor      | Template (if using one)    | `inferenceGatewayBaseUrl`                 | `inferenceGatewayAuthScheme` |
| ----------- | -------------------------- | ----------------------------------------- | ---------------------------- |
| DeepSeek    | `gateway-deepseek`         | `https://api.deepseek.com/anthropic`      | `x-api-key`                  |
| Zhipu GLM   | `gateway-glm`              | `https://open.bigmodel.cn/api/anthropic`  | `bearer`                     |
| MiniMax     | `gateway-minimax`          | `https://api.minimaxi.com/anthropic`      | `x-api-key`                  |

If you only need LLM config — no MCP, no egress lockdown — use the
per-vendor template directly:

```bash
cowork-mdm profile new --template gateway-deepseek --out lite.mobileconfig
```

Then edit the generated file to drop in the real API key (or better,
use `--from my.yaml` so the key never lands in the template). For
full enterprise scaffolding (MCP + egress + telemetry + sandbox +
auto-update policy), continue to Section 3 with `enterprise-cn-full`.

## 3. Generate the profile

### 3a. `--template` is a scaffold, not a production profile

`cowork-mdm profile new --template enterprise-cn-full --out out.mobileconfig`
will produce a valid `.mobileconfig` — but **every `REPLACE_*` string in
the output is still a placeholder**. `profile validate` will pass because
placeholders are syntactically valid strings; the profile is not yet
deployable. Do not distribute a template-emitted file to employee
machines.

### 3b. Production path — copy, fill, `--from`

The CLI's `profile new` emits `mobileconfig` or `plist` only (see
`profile new --format`), not YAML. Use `profile show-template` to dump
the YAML source of any built-in template as a starting point for your
org's overrides file:

```bash
# Dump the enterprise scaffold YAML to your private config repo
# (NOT cowork-mdm's repo).
cowork-mdm profile show-template enterprise-cn-full --out overrides.yaml

# 1. Replace every REPLACE_* placeholder in overrides.yaml with your
#    real values: gateway base URL + auth scheme + API key, model IDs,
#    MCP server list, egress allowlist, and any optional sandbox flags
#    you want to enable.

# 2. Generate the mobileconfig.
cowork-mdm profile new --from overrides.yaml --out company.mobileconfig
```

`--template` and `--from` are mutually exclusive on a single
invocation — pick one. The YAML dumped by `show-template` always
matches the CLI's own embedded copy, so there's no version drift.

### 3c. Inject the API key via MDM, not YAML

If your MDM supports secret variables (Jamf: `$MANAGED_SECRETS`, Intune:
app-config variables, Kandji: parameters), substitute them in at push
time rather than at profile-build time. Two patterns work:

1. **Pre-build substitution**: envsubst + CI. Your CI pipeline reads
   the API key from a secrets store, runs `envsubst` over
   `overrides.yaml`, produces `company.mobileconfig`, and publishes the
   file to a private artifact store the MDM pulls from.
2. **Per-machine substitution** (Jamf): leave the gateway API key blank
   in the pushed mobileconfig and use `inferenceCredentialHelper` to
   point at a shell script the MDM also pushes, which fetches the key
   per-user from your secrets service. See `cowork-mdm schema show
   inferenceCredentialHelper`.

Pattern 1 is simpler; pattern 2 avoids baking a shared secret into the
distributed mobileconfig.

## 4. Validate

```bash
cowork-mdm profile validate company.mobileconfig
```

Expected output on success: `company.mobileconfig: OK (N keys)` with a
non-zero `N`. On failure, the command prints the offending key and
exits non-zero.

**`validate` is schema-only.** It checks that keys exist in the 51-key
schema, values match their declared types, and `jsonString` fields are
parseable. It does NOT check:

- Whether any `REPLACE_*` placeholder strings remain in the output.
- Whether the vendor API key is actually valid with the vendor.
- Whether MCP servers are reachable from employee machines.
- Whether the profile is signed (required by some MDMs — see Section 5).

Run `profile lint` as a pre-distribution gate — it scans every value
in the generated profile for leftover `REPLACE_*` placeholder tokens
and exits non-zero on any finding:

```bash
cowork-mdm profile lint company.mobileconfig
# Expected on a ready-to-ship profile:
#   company.mobileconfig: no placeholder residuals
# Expected on a scaffold with unfilled values (exit 1):
#   company.mobileconfig: N placeholder(s) found — fill in before distributing:
#     inferenceGatewayApiKey: REPLACE_WITH_YOUR_API_KEY
#     ...
```

`profile lint` is narrow by design — it only flags the reserved
`REPLACE_*` convention used by the gateway-mode enterprise templates. It is NOT a
general config smell checker; older template variables like `ACCOUNT`
or `PROFILE_ID` in `bedrock-basic` are intentional slots and NOT
flagged.

## 5. Distribute via MDM

All three MDM systems follow the same two-wave pattern:

- **Wave 1**: push `company.mobileconfig` as a Custom Settings Payload,
  scoped to the target device group. This lands LLM + MCP + egress +
  telemetry + sandbox settings.
- **Wave 2**: push a Shell Script payload that runs
  `cowork-mdm marketplace add <org-plugins-repo>`. This populates
  `/Library/Application Support/Claude/org-plugins/` with your skills
  and slash commands.

Both waves should target the same device group and run on the same
cadence. Wave 2 can also run later as an update path (see Section 8).

### 5a. Jamf Pro

**Wave 1 (mobileconfig)**
1. **Settings → Computer Management → Configuration Profiles → + New**
2. **Scope**: target your Smart Group (e.g. "Claude Desktop Eligible Macs").
3. **Payload**: *Application & Custom Settings → Custom Schema →
   Upload...* Load `company.mobileconfig`. Preference Domain must be
   `com.anthropic.claudefordesktop` (the cowork-mdm encoder sets this
   automatically).
4. **Save**. Jamf distributes at the next check-in (usually ≤15
   minutes).

**Wave 2 (Script payload for plugin delivery)**
1. **Settings → Computer Management → Scripts → + New**
2. Body (assumes cowork-mdm is already present on the target Mac —
   via the `.pkg` from this repo's Releases, via brew, or via your
   own internally-signed package; see Section 1 for the install
   pre-req). The resolver checks the pkg install location first
   (`/opt/cowork-mdm/bin`), then the two brew layouts, then PATH.
   `COWORK_MDM_BIN` overrides everything for orgs with non-standard
   install paths. The script is idempotent — safe to run on the
   MDM's recurring check-in cadence:
   ```bash
   #!/bin/bash
   set -euo pipefail

   # Resolve the cowork-mdm binary across common install locations.
   # Order matches expected enterprise priority: pkg > brew (Apple
   # Silicon) > brew (Intel) > PATH. Override via COWORK_MDM_BIN.
   CLI="${COWORK_MDM_BIN:-}"
   if [ -z "$CLI" ]; then
     for candidate in \
       /opt/cowork-mdm/bin/cowork-mdm \
       /opt/homebrew/bin/cowork-mdm \
       /usr/local/bin/cowork-mdm; do
       [ -x "$candidate" ] && CLI="$candidate" && break
     done
   fi
   [ -z "$CLI" ] && CLI="$(command -v cowork-mdm || true)"
   [ -z "$CLI" ] && { echo "cowork-mdm not found" >&2; exit 127; }

   ORG_URL="https://github.com/<your-org>/claude-org-plugins"

   # Idempotent add; `marketplace add` fails if already added.
   if ! "$CLI" marketplace list --json 2>/dev/null \
        | /usr/bin/grep -q "<your-org>/claude-org-plugins"; then
     "$CLI" marketplace add "$ORG_URL"
   fi

   # Pull any new plugins the org has published since last run.
   "$CLI" marketplace update
   "$CLI" plugin list
   ```
3. **Policy → New** that runs the script, scoped to the same Smart
   Group.
4. Trigger: *Recurring Check-in* (so new plugins get picked up and the
   directory self-heals if wiped).

### 5b. Microsoft Intune for macOS

**Wave 1 (mobileconfig)**
1. **Devices → macOS → Configuration profiles → + Create profile**
2. **Profile type**: *Templates → Preference file*.
3. Upload `company.mobileconfig`. Preference domain auto-detected.
4. **Assignments**: target your macOS device group.

**Wave 2 (Shell script payload)**
1. **Devices → macOS → Shell scripts → + Add**
2. Paste the Bash body from 5a. Run as root. Frequency: *every 1 day*
   is safe (the `marketplace list | grep` guard makes it idempotent).
3. Assignments: same device group.

### 5c. Kandji

**Wave 1 (mobileconfig)**
1. **Library → Add New → Custom Profile**
2. Upload `company.mobileconfig`. Assign to Blueprint.

**Wave 2 (Custom Script)**
1. **Library → Add New → Custom Script**
2. Paste the Bash body from 5a. Execution: *Every 1 day* + *On
   Enrollment*. Assign to the same Blueprint.

### 5d. Delivering skills, slash commands, and plugin-bundled MCPs

Standalone MCP servers go in the mobileconfig via `managedMcpServers`.
But **skills, slash commands, hooks, and plugin-bundled MCPs do not
ship inside the mobileconfig.** Claude Desktop reads those from its
local `/Library/Application Support/Claude/org-plugins/` directory.
That directory is populated in one of three ways:

1. **End user runs `/plugin marketplace add <url>`** inside Claude Desktop.
2. **IT pre-populates via `cowork-mdm marketplace add <url>`** on first setup. This clones the git repo into `org-plugins/` and symlinks each plugin it finds.
3. **MDM runs `cowork-mdm marketplace add <url>` as a scripted payload** on the same cadence as the mobileconfig push. This is what Sections 5a–5c above implement.

The mobileconfig carries **LLM config** (`inferenceProvider`, gateway
URL, model list) and **standalone MCP config** (`managedMcpServers`) —
not skills, not plugins, and not plugin-bundled MCPs. `bootstrapUrl`
cannot deliver skills or plugins either: its JSON response is filtered
by a compile-time allowlist that only admits LLM and OTLP keys. See
[`docs/research/skills-plugins-mdm.md`](research/skills-plugins-mdm.md)
for the reverse-engineered evidence.

### 5e. Windows

Not yet supported end-to-end. The `.reg` encoder is tracked in
[#9](https://github.com/krislavten/cowork-mdm/issues/9); `org-plugins`
path and symlink support on Windows are tracked alongside. For now,
macOS-only fleets are the supported target.

## 6. Verify on an employee machine

After the MDM has had time to sync (15 min for Jamf, up to an hour for
Intune), SSH into a test Mac and run the operational readiness checklist:

```bash
# 1. Profile is installed at the expected scope.
#    Expected JSON: {"platform":"darwin", "present":true,
#    "targetPath":"/Library/Managed Preferences/com.anthropic.claudefordesktop.plist",
#    "profile":{...}}
cowork-mdm profile status --json | python3 -c '
import json, sys
s = json.load(sys.stdin)
if not s.get("present"):
    print("FAIL: no managed profile present")
    sys.exit(1)
print(f"OK: profile present at {s[\"targetPath\"]}")
'

# 2. No REPLACE_ placeholders leaked into the deployed plist.
#    Claude Desktop's managed plist lives at one of these two paths
#    (host-wide or per-user); lint whichever is present. Preserves the
#    failure signal so the readiness check as a whole exits non-zero if
#    any plist is dirty.
USER_PLIST="/Library/Managed Preferences/$USER/com.anthropic.claudefordesktop.plist"
HOST_PLIST="/Library/Managed Preferences/com.anthropic.claudefordesktop.plist"
LINT_FAIL=0
for f in "$USER_PLIST" "$HOST_PLIST"; do
  if [ -f "$f" ] && ! cowork-mdm profile lint "$f"; then
    LINT_FAIL=1
  fi
done
[ "$LINT_FAIL" -ne 0 ] && { echo "FAIL: placeholder residuals in deployed plist" >&2; exit 1; }

# 3. Plugins landed in org-plugins/.
#    Expected: non-empty output. Empty = Script payload (5a/b/c) didn't
#              run, or failed. See Section 7.
cowork-mdm plugin list

# 4. System health.
#    Expected: no P0 findings. P1/P2 warnings are informational.
cowork-mdm doctor
```

If all four return clean, the deployment is working. Roll to the rest
of the fleet.

## 7. Common failure modes

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| Claude Desktop shows 401 on first LLM request | `inferenceGatewayAuthScheme` mismatch with vendor (e.g. set `bearer` but vendor wants `x-api-key`) | Check Section 2 table; regenerate mobileconfig; re-push. |
| Claude Desktop can't reach MCP server | MCP host not in `coworkEgressAllowedHosts` | Add the host, regenerate, re-push. Wildcard: `"*.internal.example.com"`. |
| MCP server reachable but tool calls fail silently | MCP server unreachable from the user's sandbox (firewall, DNS) | Run `curl` from inside Claude Desktop's built-in terminal to confirm. |
| `cowork-mdm plugin list` returns empty on employee Mac | Wave 2 Script payload didn't run or `marketplace add` failed (permission denied, network block, auth on the git host) | Check the MDM's script log on the machine. Re-run manually: `sudo cowork-mdm marketplace add <url>`. Verify `/Library/Application Support/Claude/org-plugins/` is writable by root. |
| Profile not active even after MDM sync | `managedappconfigd` hasn't synced, or the Custom Settings payload targeted the wrong preference domain | Force sync: `sudo profiles sync`. Confirm domain is `com.anthropic.claudefordesktop`. |
| New profile version pushed but Claude Desktop still shows old config | Claude Desktop may cache managed config at launch (not independently verified as of v0.3) | First confirm the new plist is live: `cowork-mdm profile status --json` on the Mac should show the new values. If it does and Claude Desktop still doesn't reflect them, have the user quit and relaunch Claude Desktop. If that's a recurring issue across your fleet, you can optionally push a "Restart Claude Desktop" follow-up script after each mobileconfig update. |
| `profile validate` passes but profile is clearly broken on the Mac | Validate is schema-only — see Section 4. Placeholder string, invalid token, or MCP JSON escape issue. | Re-check YAML against actual vendor console output. |

## 8. Updating later

- **To rotate the API key or change a model list**: edit `overrides.yaml`,
  re-generate, re-validate, push as a new mobileconfig version (bump the
  `PayloadVersion` integer). MDM redelivers the new payload; Claude
  Desktop picks it up on next launch.
- **To ship new plugins / skills**: push to your plugin-marketplace git
  repo. The Wave 2 Script (Section 5) runs `marketplace update` on
  every recurring cycle, so employee machines pick up new plugins
  automatically at the next trigger — no MDM redeploy required. The
  script's idempotent guard makes it safe to run hourly if you want a
  tighter cadence.
- **To remove a plugin**: remove it from the marketplace repo; the next
  `marketplace update` run drops the symlink from employee machines.
  Run `cowork-mdm plugin prune` on the machine (or extend the Wave 2
  Script to include it) to clean up dangling entries. Users won't see a
  warning; the plugin just disappears.
- **To retire the whole deployment**: push a mobileconfig that only
  contains `disableAutoUpdates: false` (if you want updates to resume)
  and no other managed keys, plus a Script that calls `cowork-mdm
  marketplace remove <url>`. Employees drop back to a vanilla Claude
  Desktop on next launch.

---

**See also**:

- [`specs/profile.md`](../specs/profile.md) — profile-format details and
  the full `MobileConfigOpts` surface
- [`specs/schema.md`](../specs/schema.md) — the 51-key schema with full
  provenance
- [`docs/research/skills-plugins-mdm.md`](research/skills-plugins-mdm.md) — why
  skills and plugins must ship via Script payload, not mobileconfig
