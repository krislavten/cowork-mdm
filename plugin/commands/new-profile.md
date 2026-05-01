---
allowed-tools: Bash(cowork-mdm:*), Read, Write
description: Interactively author a new Claude Desktop MDM profile
---

## Context

- cowork-mdm version: !`cowork-mdm --version 2>&1 || echo "MISSING — ask user to install via brew install krislavten/tap/cowork-mdm"`
- Available templates: !`cowork-mdm profile templates 2>&1`

## Your task

Walk the user through generating a `.mobileconfig` that configures Claude
Desktop for their enterprise. Follow this flow — do **not** skip steps:

### 1. Establish intent

Ask exactly which inference provider the user wants to configure (one of
`bedrock`, `vertex`, `foundry`, `gateway`, or `mcp-only`), and whether they
additionally want to lock down MCP servers and/or egress. If the user is
unsure, route to the `mdm-profile-authoring` skill and show the provider
matrix from there.

### 2. Collect the enterprise-specific values

For each provider, ask only for the values that actually vary per org.
Don't pester for keys the template already defaults correctly. Typical
asks:

- **bedrock**: AWS region, AWS profile name, Bedrock inference-profile
  ARN list (one per model). Warn about the `[1m]` suffix for 1M-token
  variants.
- **vertex**: GCP project, region, model IDs.
- **foundry**: Azure endpoint, deployment names.
- **gateway**: base URL, auth header, model list.
- Optional for all: `managedMcpServers` (MCP server list as JSON) and
  `coworkEgressAllowedHosts` (JSON array).

Confirm whether to set `disableDeploymentModeChooser: true`. Remind that
this flag hard-gates on the inference config being complete — leaving it
off is safer for a first deploy.

### 3. Write `overrides.yaml`

Create a YAML file in the user's working directory (default
`./overrides.yaml`). `inferenceModels` and `coworkEgressAllowedHosts` are
`stringArray` keys — accept either a YAML list or a JSON-array-in-a-string
form. `managedMcpServers` is a `jsonString` — must be a single-line valid
JSON string of the shape `schema show managedMcpServers` describes. Never
put secrets in the built-in templates; the overrides YAML stays private to
the user.

### 4. Generate and validate

```bash
cowork-mdm profile new \
  --template <provider> \
  --from overrides.yaml \
  --out profile.mobileconfig
cowork-mdm profile validate profile.mobileconfig
```

If validate fails, read the error, fix the YAML, regenerate. Do not ship
an invalid profile.

### 5. Preview and hand off

Show the user the first ~40 lines of `profile.mobileconfig` so they can
spot-check. Tell them the next step is **not** local apply — it's handing
the file to their MDM (Jamf/Intune/Kandji). If they want to test locally
on this very Mac, suggest `/cowork-mdm:deploy profile.mobileconfig` which
runs a dry-run preview without touching disk.

### Rules

- Do **not** run `sudo` yourself. Ever.
- Do **not** write `profile.mobileconfig` anywhere under `/Library/…`.
- Do **not** modify files under the cowork-mdm repo's `internal/profile/
  templates/` directory. Enterprise-specific values live in the user's
  overrides YAML.
- Refer to the `mdm-profile-authoring` skill for schema/type questions
  the user raises mid-flow.
