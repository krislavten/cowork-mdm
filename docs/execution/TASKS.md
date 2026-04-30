# Tasks for v0.2

Tasks are claimed by creating `docs/execution/current_tasks/<task-id>.lock` containing the agent name + timestamp. Remove the lock when done.

GitHub Issue mapping is set up in Phase 3 by the coordinator; task IDs below match issue titles.

## Legend

- 📂 = file conflict domain (this task's files)
- 🔒 = requires coordinator PR (receive can't change)
- ⛔ = prerequisite task

## M1 — Foundation (serial)

### `task-01` — `internal/schema/`

- 📂 `internal/schema/*.go`, `internal/schema/schema.json`, `internal/schema/extract/*.go`
- 🔒 specs/schema.md, .claude/plans/v0.2.md
- ⛔ none

**Deliverables**:
- `internal/schema/schema.go` exporting `Load()`, `Find()`, `Validate()` per spec
- `internal/schema/schema.json` populated with ≥ 51 keys (coordinator provides the extracted JSON before spawn)
- `internal/schema/extract/extract.go` build-tagged `//go:build extract` — regenerates schema.json given a Claude.app path
- Unit tests: `schema_test.go`, `validate_test.go`
- `go test ./internal/schema/...` green

**Verify**:
```
docs/execution/verify.sh task-01
```

**Non-goals**: command-line wiring (done in task-11).

---

### `task-02` — `internal/paths/`

- 📂 `internal/paths/*.go`
- 🔒 specs/paths.md
- ⛔ none (can run parallel with task-01)

**Deliverables**:
- `paths.go` with `Provider` interface, `Default()`, `ForOS()`
- `paths_darwin.go`, `paths_windows.go`, `paths_other.go`
- Unit tests: table-driven per OS

**Verify**:
```
docs/execution/verify.sh task-02
```

---

### `task-03` — `internal/profile/` core + mobileconfig encoder + decoders

- 📂 `internal/profile/profile.go`, `internal/profile/encode_mobileconfig.go`, `internal/profile/decode.go`, `internal/profile/templates.go`, `internal/profile/templates/*.yaml`, `internal/profile/testdata/bedrock-*.golden.mobileconfig`, test files for owned sources
- 🔒 specs/profile.md
- ⛔ task-01

**Deliverables**:
- `Profile` struct + `New()`, `Set()`, `Get()`, `Keys()`, `Delete()`, `Validate()`. Preserves unknown keys via a side channel (see spec) so decoder warnings can surface in validate/status.
- `EncodeMobileConfig(*Profile, MobileConfigOpts)` producing valid `.mobileconfig`.
- `DecodeMobileConfig`, `DecodePlist`, `DecodeReg`, `Detect` — full decoder surface specified in specs/profile.md. Decoders return a Profile + a DecodeReport listing unknown keys / version mismatches.
- `LoadTemplate(name string)` + `TemplateNames()` + `templates/bedrock-basic.yaml` + `templates/bedrock-mcp.yaml` + `templates/gateway.yaml` + `templates/vertex.yaml` + `templates/foundry.yaml` + `templates/mcp-only.yaml` (embedded via `//go:embed`).
- Golden file `testdata/bedrock-basic.golden.mobileconfig` + `TestEncodeMobileConfigGolden`.
- Roundtrip tests with **semantic** equivalence (not byte-identical — see spec on PayloadUUID generation).
- On macOS CI: `plutil -lint` subprocess check in test.
- **Add YAML dependency**: this task is the authorized adder of `gopkg.in/yaml.v3` to `go.mod`.

**Verify**:
```
docs/execution/verify.sh task-03
```

## M2 — Encoders + managers (parallel)

### `task-04` — plist + jamf encoders

- 📂 `internal/profile/encode_plist.go`, `internal/profile/encode_plist_test.go`, `internal/profile/encode_jamf.go`, `internal/profile/encode_jamf_test.go`, `internal/profile/testdata/bedrock-basic.golden.plist`, `internal/profile/testdata/bedrock-basic.golden.jamf.plist`
- ⛔ task-03 (Profile struct + testdata infra)

**Deliverables**:
- `EncodePlist(p *Profile)` producing raw plist body
- `EncodeJamf(p *Profile)` producing Jamf Custom Settings Payload inner dict
- Golden tests for each

---

### `task-05` — intune encoder

- 📂 `internal/profile/encode_intune.go`, `internal/profile/encode_intune_test.go`, `internal/profile/testdata/bedrock-basic.golden.intune.xml`
- ⛔ task-03

**Deliverables**:
- `EncodeIntune(p *Profile)` producing Intune macOS Preference File XML wrapper per spec
- Golden tests

---

### `task-06` — reg encoder (Windows)

- 📂 `internal/profile/encode_reg.go`, `internal/profile/encode_reg_test.go`, `internal/profile/testdata/bedrock-basic.golden.reg`, `internal/profile/testdata/bedrock-basic-hkcu.golden.reg`
- ⛔ task-03

**Deliverables**:
- `EncodeReg(p *Profile, opts RegOpts)` producing CRLF-terminated `.reg` text
- All schema types represented; string arrays + json strings escaped correctly; HKLM and HKCU both tested
- Golden tests

---

### `task-07` — `internal/marketplace/`

- 📂 `internal/marketplace/*.go` + testdata
- ⛔ task-02

**Deliverables**:
- `Manager` with `Add`, `List`, `Update`, `Remove`, `LinkAll`
- go-git based (no external git CLI)
- Integration tests with local bare repo fixtures

**Verify**:
```
docs/execution/verify.sh task-07
```

---

### `task-08` — `internal/plugin/`

- 📂 `internal/plugin/*.go` + testdata
- ⛔ task-02

**Deliverables**:
- `Inspector` with `List`, `Get`, `EnabledStates`
- `Mutator` with `Unlink`, `Prune`
- Fixture-based tests with mix of real dirs, valid symlinks, dangling symlinks

---

### `task-09` — `internal/doctor/`

- 📂 `internal/doctor/*.go` + testdata
- ⛔ task-01, task-02, task-07 (uses marketplace.Manager), task-08 (uses plugin.Inspector)

**Deliverables**:
- `Runner`, `Result`, `Status`, `Check`, `Fixable` types per spec
- All checks from specs/doctor.md implemented (those not applicable on a given OS return StatusSkipped)
- Org-plugin checks **reuse** `internal/marketplace/` and `internal/plugin/` APIs (no duplicate symlink/discovery logic)
- `Exit()` helper + `JSON()` serializer
- Unit tests per check

---

### `task-apply` — `internal/managed/` (apply/status side-effects)

- 📂 `internal/managed/*.go` + testdata
- ⛔ task-02 (paths), task-03 (Profile + encoders + decoders)

**Deliverables**:
- `Apply`, `Status`, `ApplyOptions`, `ApplyResult`, `StatusReport` types per specs/managed.md
- Platform-specific apply_darwin.go / apply_windows.go / apply_other.go + status_darwin.go / status_windows.go / status_other.go
- Sentinel errors: `ErrPermission`, `ErrUnsupportedPlatform`
- Tests use `TargetPath` override to avoid touching real `/Library/Managed Preferences/`

## M3 — UI + wiring

### `task-10` — `internal/ui/` TUI wizard

- 📂 `internal/ui/*.go`
- ⛔ task-03

**Deliverables**:
- `RunWizard()` entry returning a constructed `*Profile`
- bubbletea model covering: provider select → provider-specific fields → MCP template pick → review → save/abort
- Non-TTY detection: return error with hint ("use --template instead") if stdin/stdout isn't a TTY

---

### `task-11` — `cmd-wiring/` + `cmd/cowork-mdm/main.go`

- 📂 `cmd-wiring/*.go`, `cmd/cowork-mdm/main.go`
- ⛔ task-03, task-04, task-05, task-06, task-07, task-08, task-09, task-10, task-apply

**Deliverables**:
- `main.go` wires cobra root command with version/--json/--verbose/--no-color
- subcommands for `schema`, `profile`, `marketplace`, `plugin`, `doctor` per specs/cli.md
- `profile apply` / `profile status` call into `internal/managed/` — no direct filesystem or registry writes from the cmd layer
- Exit codes match spec
- Help text matches spec (assertions in tests)

## M4 — Release plumbing

### `task-ci` — GitHub Actions CI

- 📂 `.github/workflows/ci.yml`
- ⛔ task-11 (need a compilable binary to lint/test)

**Deliverables**:
- CI matrix: darwin-14 / windows-2022 / ubuntu-24.04
- Steps: `go build ./...`, `go test ./... -race`, `golangci-lint run`
- Artifact uploaded only on main branch merges (binary for manual sanity)

---

### `task-release` — GoReleaser + Homebrew tap

- 📂 `.goreleaser.yaml`, `.github/workflows/release.yml`
- ⛔ task-ci

**Deliverables**:
- `.goreleaser.yaml` produces darwin {amd64, arm64} + windows {amd64, arm64} + linux {amd64, arm64} archives
- Homebrew formula auto-push to `krislavten/homebrew-tap`
- Release workflow triggered on `v*` tag

---

### `task-docs` — README + deployment cookbook

- 📂 `docs/*.md` (excluding `docs/execution/`)
- ⛔ task-11

**Deliverables**:
- `docs/macos-mdm.md`: Jamf/Kandji/Intune walkthrough using cowork-mdm
- `docs/windows-gpo.md`: reg file + GPO + Intune
- `docs/marketplace.md`: how to add / update / prune
- `docs/troubleshooting.md`: common doctor red flags + remedies
- README updated with final command surface + brew tap instructions

## File conflict quick reference

- task-01: `internal/schema/**`
- task-02: `internal/paths/**`
- task-03: `internal/profile/profile.go`, `internal/profile/encode_mobileconfig.go`, `internal/profile/decode.go`, `internal/profile/templates.go`, `internal/profile/templates/*.yaml`, `internal/profile/testdata/bedrock-basic.golden.mobileconfig`, and tests for these sources
- task-04: `internal/profile/encode_plist.go`, `internal/profile/encode_jamf.go`
- task-05: `internal/profile/encode_intune.go`
- task-06: `internal/profile/encode_reg.go`
- task-07: `internal/marketplace/**`
- task-08: `internal/plugin/**`
- task-09: `internal/doctor/**`
- task-10: `internal/ui/**`
- task-apply: `internal/managed/**`
- task-11: `cmd/**`, `cmd-wiring/**`
- task-ci: `.github/workflows/ci.yml`
- task-release: `.goreleaser.yaml`, `.github/workflows/release.yml`
- task-docs: `docs/*.md` (outside `docs/execution/`)

**Always forbidden for task agents**:
- `.claude/plans/**`
- `specs/**`
- `AGENTS.md`
- `docs/execution/TASKS.md` (only mark `[x]`, no other edits)
- `docs/execution/verify.sh`
- `internal/schema/schema.json` after M1 freeze
- `go.mod` / `go.sum` (only via `go get` on owned task's domain; conflicts → coordinator)
