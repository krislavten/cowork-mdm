---
name: mdm-profile-authoring
description: Authoring Claude Desktop Managed Preferences profiles — selecting the right inference provider (Bedrock, Vertex, Azure Foundry, generic gateway, or MCP-only), looking up schema keys, writing a profile YAML, generating .mobileconfig or .plist, and validating the result. Load when the user wants to create, edit, or inspect a Claude Desktop MDM config — phrases like "write a mobileconfig", "configure Bedrock/Vertex/Azure for Claude", "lock down MCP servers", "what keys are available", "profile new", "profile validate", or questions about specific MDM keys like inferenceProvider, inferenceModels, managedMcpServers, coworkEgressAllowedHosts.
---

# MDM profile authoring

This skill covers **creating and validating** a Claude Desktop MDM profile.
For deploying the profile to real hosts, switch to `mdm-profile-deploy`.

## The schema in 30 seconds

Claude Desktop reads ~51 MDM keys from a managed plist (macOS) or registry
(Windows). Anthropic documents 8 publicly; `cowork-mdm` embeds the full set
extracted from the Electron bundle's zod schema.

Every key has a **type** (`string`, `bool`, `integer`, `stringArray`,
`jsonString`), a **scope** (`3p`, `3p-bootstrap`, …), and an `appMin`
(minimum Claude Desktop version that reads it). Two quirks to internalize:

- **`stringArray`** — plain list of strings. The YAML side accepts either a
  native list or a JSON-array-in-a-string (`'["a","b"]'`); the CLI coerces
  to `[]string` at load time. Keys of this type include `inferenceModels`
  and `coworkEgressAllowedHosts`. On disk they render as `<string>` entries
  wrapping a JSON array (that's how the live Claude Desktop app stores them
  too — do not try to "fix" this into a plist `<array>`).
- **`jsonString`** — keys typed as *a string whose contents are valid JSON
  of a specific inner shape*. The plist holds a single `<string>`; the app
  JSON-parses it at read time. `managedMcpServers` is the big one. When
  hand-editing, the inner JSON must stay a single line and be valid.

When in doubt, `cowork-mdm schema show <key> --json` tells you the exact
type and example. Do not guess a key's type from its name — `inferenceModels`
is `stringArray`, not `jsonString`.

> **Safety**: this skill only **authors** profiles. Do not `sudo`, do not
> write to `/Library/Managed Preferences/`, and do not run
> `cowork-mdm profile apply` without `--dry-run` on the user's behalf. If
> the user pivots from "write this profile" to "now apply it", hand off
> to the `mdm-profile-deploy` skill — it enshrines the MDM-channel rule.

## Canonical authoring workflow

```
1. schema list [--json]                 # orient (--json is stable, table output is for humans)
2. schema show <key> [--json]           # drill into each key
3. profile templates                    # pick a starter
4. Write overrides.yaml                 # your enterprise values (secrets here, NOT in template dir)
5. profile new --template X --from overrides.yaml --out out.mobileconfig
6. profile validate out.mobileconfig    # gate — succeeds silently, fails loud with exit 1
```

`profile validate` prints `FILE: OK (N keys)` on success; on failure it
prints the offending key and exits non-zero. The output is human-readable
text, not JSON — for programmatic checks, rely on the exit code.

**Never edit the template files under the repo.** They're shipped in the
binary and are meant to be provider-neutral scaffolds. Enterprise-specific
values (ARNs, MCP tokens, allowed-host lists) belong in a **user YAML file**
that stays private.

## The five built-in templates

`cowork-mdm profile templates` prints the current list. As of v0.3:

| Template | Inference provider | Typical customization |
| --- | --- | --- |
| `bedrock-basic` | AWS Bedrock via `~/.aws` | `inferenceBedrockRegion`, `inferenceBedrockProfile`, `inferenceModels` (ARN `stringArray`) |
| `vertex` | Google Vertex AI | project id, region, model IDs |
| `foundry` | Azure Foundry | endpoint, deployment names |
| `gateway` | Generic OpenAI-compatible gateway (LLM proxy) | base URL, auth header, model list |
| `mcp-only` | No inference override, only locks MCP + egress | `managedMcpServers`, `coworkEgressAllowedHosts` |

## Overrides YAML shape

The `--from` file uses the same structure as a template:

```yaml
name: my-org-bedrock        # used as PayloadDisplayName in mobileconfig
description: |
  Optional. Shown in MDM UIs.
values:
  inferenceProvider: bedrock
  inferenceBedrockRegion: us-west-2
  inferenceBedrockProfile: default
  inferenceBedrockAwsDir: /Users/{user}/.aws   # or leave default
  # stringArray keys: either a YAML list or a JSON-array-as-string work.
  # The CLI coerces both to []string. The live Claude Desktop app stores
  # these on disk as <string>["..."]</string>, so don't be surprised.
  inferenceModels: >-
    ["arn:aws:bedrock:us-west-2:ACCOUNT:application-inference-profile/OPUS_ID","arn:aws:bedrock:us-west-2:ACCOUNT:application-inference-profile/SONNET_ID[1m]","arn:aws:bedrock:us-west-2:ACCOUNT:application-inference-profile/HAIKU_ID"]
  coworkEgressAllowedHosts: '["*.internal.example.com","api.example.com"]'
  # jsonString key: must be a valid-JSON string of the shape the schema
  # describes (object array with name/url/transport/toolPolicy/...).
  managedMcpServers: >-
    [{"name":"jira","url":"https://mcp.example.com/jira","transport":"http"}]
  disableDeploymentModeChooser: true
```

Key points:

- YAML `>-` ("folded, strip") joins wrapped lines into a single string
  without a trailing newline. Use it for long inline JSON.
- Booleans go raw (`true` / `false`), not quoted.
- Overrides **replace** the template's value for that key; templates and
  overrides merge at the key level.

## The `[1m]` suffix on Bedrock ARNs

Some Bedrock accounts are entitled to 1M-token context variants of a model.
The app detects these by the `[1m]` suffix inside the last path segment of
the ARN:

```
…inference-profile/v37lj7n5l53w[1m]
```

If you're **not** entitled, include the base ARN without the suffix. If you
are, the suffix gives users a 1M-context picker entry.

## Generating and validating

```bash
cowork-mdm profile new \
  --template bedrock-basic \
  --from overrides.yaml \
  --out /tmp/cowork.mobileconfig

cowork-mdm profile validate /tmp/cowork.mobileconfig
```

`profile validate` checks:

- every key exists in the embedded schema (no typos);
- every value matches its key's type (no `boolean` in a `string` slot);
- `jsonString` keys contain parseable JSON of the declared inner shape.

If validation fails, the message tells you the offending key. Fix the YAML,
regenerate, re-validate.

## Round-trip: reading an existing profile

There is no `profile decode` subcommand yet. For v0.3, the easiest round-
trip check is to **apply the authored profile to the host (or any test
host) and read it back with `cowork-mdm profile status --json`**. That
command decodes the live plist into `profile.values` — compare that
key/value map against the YAML you authored.

## Quick provider recipes

### Bedrock (most common for 3p deployments)

```yaml
values:
  inferenceProvider: bedrock
  inferenceBedrockRegion: us-west-2
  inferenceBedrockProfile: default
  inferenceModels: >-
    ["arn:aws:bedrock:us-west-2:ACCOUNT:application-inference-profile/OPUS","arn:aws:bedrock:us-west-2:ACCOUNT:application-inference-profile/SONNET[1m]"]
  disableDeploymentModeChooser: true
```

### Vertex AI

```yaml
values:
  inferenceProvider: vertex
  inferenceVertexProjectId: my-gcp-project
  inferenceVertexRegion: us-central1
  inferenceModels: >-
    ["claude-opus-4-7@20260101","claude-sonnet-4-6@20260101"]
  disableDeploymentModeChooser: true
```

(Double-check field names with `cowork-mdm schema list --json | jq
'.[].name' | grep -i vertex` — Vertex has several adjacent keys like
`inferenceVertexCredentialsFile`, `inferenceVertexOAuthClientId`, etc.)

### Locked-down MCP + egress without overriding inference

```yaml
values:
  managedMcpServers: >-
    [{"name":"confluence","url":"https://mcp.internal/confluence","transport":"http"}]
  coworkEgressAllowedHosts: '["*.internal.example.com"]'
```

Always confirm the exact field names with `schema show` before shipping — the
zod schema is the source of truth, not this document.

## Gotchas

- **`coworkEgressAllowedHosts` is a `stringArray`** (per the zod schema),
  but the live Claude Desktop plist stores it on disk as a single
  `<string>` wrapping a JSON array. The CLI emits whichever shape matches
  the app's actual behavior; don't try to "normalize" it yourself. `["*"]`
  means "allow everything."
- **`inferenceBedrockAwsDir` is absolute, per-user**. Either set a fixed
  path (`/Users/<user>/.aws`) and accept that only that user's machine
  works, or leave it unset and let Claude Desktop default to `~/.aws`.
- **MCP tokens in `managedMcpServers` are readable by anyone with plist
  read access.** They belong in overrides, not the shared template dir, and
  if your threat model cares, rotate via MDM push rather than embedding.
- **`profile new --format plist` emits deterministic bytes.** Same inputs,
  same bytes — verified to byte-match the live maintainer plist after
  plutil normalization. The `mobileconfig` format does NOT: it includes a
  fresh `PayloadUUID` per run. Compare decoded key/value sets via
  `plutil -convert json` (or `cowork-mdm profile status`) rather than raw
  bytes when validating round-trip.
