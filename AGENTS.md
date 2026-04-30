# AGENTS.md — cowork-mdm 开发规约

给所有参与本仓库开发的 AI agent(和人类)看。最后更新:v0.2 swarm 启动前。

---

## 本次 swarm 的关键信息

- **Plan**: `.claude/plans/v0.2.md` — 战略方向 + task 拆分 + milestone
- **Specs**: `specs/*.md` — 每个包的接口契约
- **Tasks**: `docs/execution/TASKS.md` — 任务清单 + 文件冲突域 + 验证步骤
- **Verify**: `docs/execution/verify.sh <task-id>` — 提交前门禁(Sparring + CI 也靠它)
- **Progress**: `docs/execution/progress/<your-agent-name>.md` — 你的进度日志
- **Locks**: `docs/execution/current_tasks/<task-id>.lock` — 认领任务的文件锁

---

## 铁律(违反即 block PR)

### 1. Spec-First + Sparring Review

所有代码变更、Spec 变更在 commit 前必须经过 Sparring Review。

| 必须 Sparring | 不需要 Sparring |
|---|---|
| 代码变更(任何规模) | 纯事实陈述 |
| Spec 新增/修改 | 提问澄清 |
| 技术方案结论 | 中间过程状态更新 |

**Sparring 命令**(本项目默认用 Codex 的 rescue 子 agent):

```bash
# Coordinator 执行(agent 在自己 session 中调用):
/codex:rescue
# 贴 diff + plan/spec 路径 + 本任务的 scope 边界
```

输出格式:
- `APPROVE` — 通过
- `CONCERNS` — 每条标 `MUST-FIX` / `SHOULD-FIX` / `NIT`

处理规则:
- MUST-FIX → 必修,修完重跑
- SHOULD-FIX → 评估修或标为已知
- NIT → 可选
- 5 轮未过升级给 coordinator

**跳过 Sparring 直接 commit = 违反铁律。**

### 2. 提交前 5 步门禁

```
修改代码
  ↓
1. gofmt / go vet → 失败修复
  ↓
2. go build ./... → 失败修复
  ↓
3. GOOS=darwin/windows/linux go build ./... → 跨编译通过
  ↓
4. docs/execution/verify.sh <task-id> → 跨编译 + unit test 全过
  ↓
5. /codex:rescue 审查 diff → APPROVE
  ↓
可以 commit + push
```

### 3. 测试强制覆盖

每个导出函数/方法必须有单元测试。与 Sparring 同级铁律。

| 类别 | 覆盖要求 |
|---|---|
| 纯函数(encode/decode/validate) | 正常 / 边界 / 非法输入 |
| 路径/平台差异(paths) | 每个 OS 的期望输出 |
| 带副作用(marketplace/plugin) | 使用 tempdir fixtures,无网络 |
| 诊断(doctor) | 每个 Check 独立测,加 orchestrator 集成测 |

测试必须和代码在**同一个 commit**。"先提代码后补测试" = 违规。

### 4. Worktree 隔离

多 agent 并行时每人独立 worktree:

```bash
git worktree add -b feat/task-N /tmp/agent-wt/<agent-name> origin/main
```

**每次 Bash 调用前缀 `cd /tmp/agent-wt/<agent-name> &&`**,不能假设 cwd 保留。

主 repo `/Users/kris/develop/cowork-mdm` 是 coordinator 专属。

### 5. 受保护文件

以下文件只有 **coordinator** 可改(通过独立 chore PR):

- `.claude/plans/**`
- `specs/**`
- `AGENTS.md`
- `docs/execution/verify.sh`
- `docs/execution/TASKS.md` (agent 可勾自己的 `[x]`,但不能改描述)
- `internal/schema/schema.json` (v0.2 冻结后)

需要修改 → 在你的 progress log 里写下诉求 + SendMessage coordinator,不要私改。

---

## Go 项目约定

### Module path

`github.com/krislavten/cowork-mdm`

### 依赖限制

- 最小化外部依赖。加新依赖要在 Sparring 中说理由。
- 避免 cgo — 保持纯 Go 跨编译
- Go 版本: 1.23(`go.mod` 固定)

### Package 命名

- `internal/<package>/` — 不对外暴露的业务逻辑
- `cmd/cowork-mdm/` — 二进制入口(`main.go`)
- `cmd-wiring/` — cobra subcommand wiring,依赖所有 `internal/`
- `internal/<pkg>/extract/` 等 — 带 build tag 的维护工具

### 文件布局

```go
package foo

// 文件顺序:
// 1. 包注释(可选,特别是 public package)
// 2. 常量
// 3. 类型定义(公开优先)
// 4. 构造函数(New* / Default*)
// 5. 方法(按接收者分组)
// 6. 自由函数
// 7. 内部辅助(小写)
```

### 错误

- 用 `fmt.Errorf("foo: %w", err)` 保留 wrap
- 导出的 sentinel 错误:`var ErrFoo = errors.New("foo: ...")`
- 不用 panic,除非真的是"程序逻辑保证不会发生"的 invariant 破坏

### 测试

- Table-driven 优先
- 使用 `t.TempDir()` 构建 fixture 目录
- 没有 live-system 依赖(不读当前用户的 `/Applications/Claude.app`,用 `testdata/` fixture)
- Golden files 存在 `testdata/<case>.golden.<ext>`,更新用 `-update` flag
- 测试文件都带 `_test.go` 后缀,同包测试优先,`_test` 包用于跨 package

### 并发 / 生命周期

- CLI 工具,不启 goroutine 做 worker pool
- 唯一并发点:`marketplace.UpdateAll` 可以并行 pull 多个 repo(用 `errgroup`)

---

## Plan vs Spec

- **Plan** (`.claude/plans/v0.2.md`) — 实施方案,包含 milestone + task 拆分。生命周期:v0.2 完成后归档。
- **Spec** (`specs/*.md`) — 设计决策,长期维护。v0.3 会有新 spec,但 v0.2 spec 是契约。

Spec 写:
- 公开 API 签名
- 算法 / 协议 / 状态机
- 类型映射(如 schema 的 plist XML 映射)
- 非目标(out-of-scope 清单)

Spec 不写:
- 具体代码实现
- 测试用例
- 细碎注释

Spec 变更必须 Sparring + 走 coordinator chore PR。

---

## Issue/PR 流程

1. 认领任务: 创建 `docs/execution/current_tasks/<task-id>.lock`,内容 = 你的 agent name + ISO8601 时间戳
2. 开 worktree + 分支: `git worktree add -b feat/task-N /tmp/agent-wt/<your-name> origin/main`
3. 在 worktree 里开发
4. 跑 `docs/execution/verify.sh task-N`
5. Sparring review
6. Commit + push + `gh pr create --title "task-N: <subject>" --body "Closes #<issue>"`
7. PR merged 后:删 lock,在 `docs/execution/progress/<your-name>.md` 追加 handoff(如果还有后续 task)

PR 命名: `task-01: add internal/schema` — task id 必须在标题里,方便 coordinator grep。

---

## Coordinator 何时介入

你可以(应该)主动 `SendMessage coordinator` 的场景:

- 发现 spec 和实际需求有 drift → 请求 coordinator 开 chore PR 改 spec
- 两个 task 的文件域意外重叠(plan 没覆盖到的情况) → 请求 scope 仲裁
- 受保护文件需要修改
- Sparring 第 4 轮还没过 → 升级
- 外部工具问题(go-git 罢工 / GitHub Actions 权限不足)

不要在没同步 coordinator 的情况下自行决定跨 task 的事。
