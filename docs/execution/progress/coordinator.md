# Coordinator progress log

## 2026-04-30 — Swarm bootstrap completed

### Infrastructure delivered

- Repo created: [krislavten/cowork-mdm](https://github.com/krislavten/cowork-mdm)
- Homebrew tap: [krislavten/homebrew-tap](https://github.com/krislavten/homebrew-tap) (empty Formula/ dir, GoReleaser will populate)
- Go 1.23 scaffold + LICENSE (MIT) + README + `.gitignore`
- CI pipeline on 3 platforms (darwin/linux/windows, amd64+arm64 cross-build), lint, race tests — green on PR #2
- GoReleaser v2 config (6 platform binaries + Homebrew formula auto-push)
- Minimum `cmd/cowork-mdm/main.go` stub so `go build ./...` is green
- 51-key `internal/schema/schema.json` seeded from Claude.app 1.5354.0

### Planning artifacts

- `.claude/plans/v0.2.md` — strategic plan (Sparring-approved after 3 rounds)
- `specs/{schema,paths,profile,marketplace,plugin,doctor,managed,cli}.md` — 8 specs
- `docs/execution/TASKS.md` — 13 tasks with file domains + verification
- `docs/execution/verify.sh` — pre-commit gate (staged + working tree + committed diff aware)
- `AGENTS.md` — Sparring rules, 5-step commit gate, worktree isolation

### Sparring trail

| Round | Result | Notes |
|---|---|---|
| Plan+specs R1 | CONCERNS (16 issues) | 9 MUST-FIX surfaced: decoder contract, apply/status indirection, Intune format, UUID roundtrip, etc. |
| Plan+specs R2 | CONCERNS (7 MUST) | First fix pass missed some stale refs |
| Plan+specs R3 | APPROVE | All MUST resolved |
| CI infra | CONCERNS (2 SHOULD + 1 NIT) | Fixed tidy-check & dropped broken artifact upload |
| Schema seed | not reviewed (file is data) | Sanity checked via jq + key spot-checks |

### GitHub Issues (task → issue mapping)

| Task | Issue |
|---|---|
| task-01 internal/schema/ | #4 |
| task-02 internal/paths/ | #5 |
| task-03 internal/profile/ core + decoders + templates + mobileconfig | #6 |
| task-04 plist + jamf encoders | #7 |
| task-05 intune encoder | #8 |
| task-06 reg encoder | #9 |
| task-07 internal/marketplace/ | #10 |
| task-08 internal/plugin/ | #11 |
| task-09 internal/doctor/ | #12 |
| task-10 internal/ui/ TUI | #13 |
| task-apply internal/managed/ | #14 |
| task-11 cmd-wiring + main.go | #15 |
| task-docs | #16 |

### Remaining work

- **M1 (serial)**: task-01, task-02 can run in parallel (no deps); task-03 waits on task-01
- **M2 (parallel after M1)**: task-04, task-05, task-06, task-07, task-08, task-09, task-apply all unblocked after task-03 (plus task-07/08 only need task-02)
- **M3**: task-10 unblocked after task-03; task-11 waits on M2 + task-apply + task-10
- **M4**: task-docs waits on task-11, then tag v0.2.0

### Open risks

1. **Coordinator review throughput**: with ~12 task PRs + chore PRs in flight, Sparring rounds + merge decisions are the bottleneck. Cap parallel active agents at 2-3 to keep pace.
2. **Windows CI on task-07/task-08**: these are macOS-only features but the test suite runs across all 3 OSes. Tests must use build tags (`//go:build darwin`) or stub cleanly on non-macOS.
3. **schema.json drift**: if Anthropic ships Claude.app 1.6.x mid-v0.2, ignore. v0.3 addresses auto-re-extraction.
4. **HOMEBREW_TAP_GITHUB_TOKEN not yet set**: before tagging v0.2.0, maintainer must `gh secret set` it. Documented in README → Maintainer notes.

### Next session pickup

To resume:
1. `cd /Users/kris/develop/cowork-mdm && git pull`
2. Check `gh pr list --state open` and `gh issue list --state open`
3. Read latest entry in this file
4. Start M1: spawn first agent for task-01 (issue #4) OR task-02 (issue #5), or both in parallel (no file conflict).

Agent spawn template: see TASKS.md task descriptions + `~/.claude/workflows/agent-swarm/templates/agent-prompts/first-agent.md` (adapt the PNPM-specific bits for Go — replace `pnpm build` with `go build ./...`, etc.).

---

## 2026-05-01 — v0.1.0 shipped; swarm paused; M2+ deferred

### What changed

- **task-01 (#4) merged** — internal/schema/ with 51-key embedded schema, Load/Find/Validate, extract maintainer tool. PR #18. Sparring round: 2 SHOULD-FIX, fix #1 (Key.Default) applied in-tree, fix #2 deferred.
- **task-02 (#5) merged** — internal/paths/ platform-aware resolver. PR #19. Sparring round: 1 MUST-FIX (HOME="/" edge case producing relative path), fixed in-tree.
- **v0.1.0 released** — PR #20 added cobra wiring + `schema list/show` + `paths show [--os]` subcommands, tests. PR #21 gated homebrew publish on HOMEBREW_TAP_GITHUB_TOKEN (skip_upload template), unblocking the release after the first push failed on 401.
- **GoReleaser shipped 6 platform archives + checksums** to https://github.com/krislavten/cowork-mdm/releases/tag/v0.1.0 .

### Why v0.2 paused

After task-01 + task-02 merged, I tried to spawn agents for task-03 (profile core), task-07 (marketplace), task-08 (plugin) as three parallel workers. All three stalled at the "reading spec" step with no filesystem writes, tripped by the 600s stream watchdog. Two earlier agents (task-01, task-02) had also stalled mid-flow; I hand-recovered both by running verify + Sparring + commit myself.

Five consecutive subagent stalls on this project, zero successful end-to-end subagent completions, suggests the current runtime is not suited for long-running multi-step subagent tasks that read ~2000 lines of specs and then write ~1000 lines of Go.

**Decision (with user): collapse v0.2's ambition to a v0.1 ship-now + defer v0.2 to classic single-session incremental builds.** Each future session does one task (maybe two if small), in the coordinator's own worktree, no subagent tier.

### Next session pickup

1. `git pull`
2. `gh pr list --state open` (should be empty)
3. `gh issue list --state open` (should be 11 — tasks 03..docs)
4. Pick one task from docs/execution/TASKS.md. Start with **task-03** (profile core) — gates task-04, 05, 06, apply, 10, 11. See specs/profile.md (has all the context you need — no need to re-read everything).
5. Work directly in `/Users/kris/develop/cowork-mdm` (no worktree needed unless paralleling), use feat/task-03 branch.
6. Follow the standard commit gate: gofmt / vet / cross-build / verify.sh task-03 / /codex:rescue APPROVE / commit / push / PR / CI / merge.
7. Once task-03 merges, next session can do task-04 OR task-07/task-08/task-apply (any M2 task that only needs task-02+03).

### Install your own v0.1.0

```bash
# macOS arm64:
curl -L https://github.com/krislavten/cowork-mdm/releases/download/v0.1.0/cowork-mdm_0.1.0_darwin_arm64.tar.gz | tar -xz
./cowork-mdm schema show inferenceProvider
./cowork-mdm paths show --os darwin
```

### Unresolved

- `HOMEBREW_TAP_GITHUB_TOKEN` still not set — `brew install krislavten/tap/cowork-mdm` won't work until that's configured on the repo.
- GitHub Actions deprecation warnings for Node.js 20 actions (non-blocking, deadline June 2026). Upgrade when we next touch .github/workflows/.

