# cowork-mdm

> **面向企业集群下发 Claude Desktop —— 包括 Anthropic 公开文档之外的那 43 个 MDM 键。** Claude Desktop 是 Electron 应用，其内嵌 zod schema 定义了 51 个 managed-preferences 键；Anthropic 的[公开企业文档](https://support.anthropic.com/en/articles/12188074)只覆盖其中 8 个。剩下 43 个键，才是你实际把桌面端指向 Bedrock、Vertex、Foundry、Anthropic-兼容网关（承接 DeepSeek / Qwen / GLM / MiniMax / Llama / Mistral）或自托管 vLLM / SGLang 集群所需要的，同时还要能锁死出口、MCP、遥测、沙箱和自动更新策略。`cowork-mdm` 从应用包里把完整 schema 提出来，生成 `.mobileconfig`，下发前 lint payload，下发后做主机级诊断。

[English](README.md) · **中文**

<p align="center">
  <a href="#快速开始"><img alt="Quickstart" src="https://img.shields.io/badge/quickstart-6%20commands-green?style=flat-square" /></a>
  <a href="#claude-code-插件"><img alt="Claude Code plugin" src="https://img.shields.io/badge/claude%20code%20plugin-5%20skills%20%2B%204%20commands-black?style=flat-square" /></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" /></a>
</p>

**状态**：**v0.3 —— CLI + Claude Code 插件。** Schema 当前锁定在 Claude.app 1.5354.0。macOS 端到端支持；Windows `.reg` 编码器仍在 [#9](https://github.com/krislavten/cowork-mdm/issues/9) 跟踪。**与 Anthropic 无官方关联** —— 以下一切均基于对公开 Claude Desktop 应用的逆向分析。

---

## 为什么需要它

Anthropic 的 Claude Desktop 是个 Electron 应用。打开应用包，在 UI 代码旁边，藏着一段内嵌 zod schema，声明了**渲染器启动时会读取的每一个 MDM 键**：LLM provider、网关 URL、鉴权方式、模型白名单、托管 MCP 服务、出口白名单、遥测端点、沙箱约束、自动更新策略、凭据助手脚本 —— **合计 51 个键**。

Anthropic 的[公开企业文档](https://support.anthropic.com/en/articles/12188074)覆盖其中 **8 个**。剩下 43 个 —— 也就是当你并不打算把 Claude Desktop 指向 `api.anthropic.com` 时真正需要的那些 —— 只作为字符串藏在一段压缩后的 Electron chunk 里。

企业 IT 每次尝试做以下事情时都会撞上这堵墙：

- 把桌面端指向**开放权重模型家族或兼容厂商**（DeepSeek、Qwen、Zhipu GLM、MiniMax、Llama、Mistral 等）通过 Anthropic-兼容接口提供的服务。
- 指向**云厂商托管路径** —— Bedrock / Vertex / Foundry 上托管的 Claude 或其他模型。
- 指向**自托管 vLLM / SGLang** 集群，走同一套 Anthropic-兼容适配。
- 在集群规模上锁死**出口**、**托管 MCP**、**遥测**、**沙箱**或**自动更新**策略 —— 这些都不在公开文档里。
- 把**公司技能、斜杠命令、插件内置 MCP** 下发到每一台员工 Mac —— 而 mobileconfig 通道在设计上就拒绝传递这些（[证据](docs/research/skills-plugins-mdm.md)）。

`cowork-mdm` 是这段企业故事缺失的另一半：一个 CLI，把完整 schema 提出来，按 schema 生成正确的 MDM 配置，在你把文件交给 Jamf / Intune / Kandji 之前 lint 一下，然后在员工机真出问题时做诊断。

## 全景一览

| | 你能得到什么 |
|---|---|
| **Schema** | 全部 51 个 managed-preferences 键，从应用内嵌 zod schema 逐字抽取（锁定在 Claude.app 1.5354.0）。通过 `cowork-mdm schema list` 和 `schema show <key>` 查询：名称、类型、作用域、`appMin`、允许值、示例。 |
| **模板 (9 个)** | `gateway` · `gateway-deepseek` · `gateway-glm` · `gateway-minimax` · `bedrock-basic` · `vertex` · `foundry` · `enterprise-cn-full`（一站式企业脚手架：LLM + MCP + 出口 + 遥测 + 沙箱 + 自动更新） · `mcp-only` |
| **配置编写** | `profile show-template NAME --out overrides.yaml` → 编辑 → `profile new --from overrides.yaml --out company.mobileconfig`。`--template` 与 `--from` 刻意互斥 —— 脚手架不是生产配置。 |
| **Lint 作为下发前门禁** | `profile validate` 只做 schema 校验 —— 它把 `REPLACE_WITH_YOUR_API_KEY` 当作合法字符串接受。`profile lint` 扫描成品里残留的 `REPLACE_*` 占位符并以非零退出。两者都要跑，不是二选一。 |
| **MDM 下发手册** | Jamf Pro · Microsoft Intune · Kandji —— 完整的 Custom Settings payload + Shell Script payload 配方见 [`docs/deployment-cn.md`](docs/deployment-cn.md)。 |
| **插件下发 (macOS)** | `marketplace add <repo>` 把 Claude-Code 格式的插件市场 clone 到 `/Library/Application Support/Claude/org-plugins/` 并为每个插件建立 symlink。设计上就是和 mobileconfig 下发同一批 push 里跑的 Shell Script payload。 |
| **主机级诊断** | macOS 上 `cowork-mdm doctor` 跑 9 项检查，覆盖应用安装、活跃 plist、org-plugins symlink、市场仓库健康、用户会话、git 可用性；带 `--fix` 选项自动修复可安全修复的项。Windows 在 [#9](https://github.com/krislavten/cowork-mdm/issues/9) 完成前只跑更小的子集。 |
| **Claude Code 插件** | 5 个技能 + 4 个斜杠命令，让 Agent 在你 session 里直接驱动 CLI：`/deploy`、`/new-profile`、`/doctor`、`/refresh-plugins`。**不包含任何新逻辑**；依赖 `PATH` 上的 CLI。 |
| **平台** | macOS（完整），Windows（`.reg` 编码器在 [#9](https://github.com/krislavten/cowork-mdm/issues/9) 跟踪）。 |
| **License** | MIT。 |

## 快速开始

```bash
brew install krislavten/tap/cowork-mdm

# 企业 gateway 部署的 6 步标准路径：
cowork-mdm profile show-template enterprise-cn-full --out overrides.yaml
$EDITOR overrides.yaml                           # 填完每一个 REPLACE_* 占位符
cowork-mdm profile new --from overrides.yaml \
  --payload-identifier-prefix com.acme.it \
  --out company.mobileconfig
cowork-mdm profile lint company.mobileconfig    # 下发前必须先过（退出 0）
cowork-mdm profile validate company.mobileconfig
# 通过 Jamf / Intune / Kandji 下发 company.mobileconfig。
# 要同时下发 技能 / 斜杠命令 / 插件内置 MCP，再加一个 Script payload
# 跑 `cowork-mdm marketplace add <your-org-plugins-repo>`。
```

走云厂商托管路径 (Bedrock / Vertex / Foundry)，把模板名换成 `bedrock-basic` / `vertex` / `foundry`，填入 `{{ACCOUNT}}` / region / 模型 ID 占位符即可，下游流水线完全一致。

**完整手册**：[`docs/deployment-cn.md`](docs/deployment-cn.md) —— 八节覆盖前置准备、provider 选择、配置生成、lint + validate 门禁、MDM 下发（Jamf + Intune + Kandji，含插件的 Script payload）、员工机验证、常见失败场景、后续更新。

## 四个核心设计原则

### 1 · Schema 即真理之源 —— 而它在应用包里，不在文档里

Claude Desktop 是 Electron 应用。它内嵌的 zod schema 声明了渲染器启动时会读取的每一个 MDM 键：名称、类型、作用域、`appMin`、允许值。我们把这段 schema 逐字抽出来，钉死到一个具体的 Claude.app 版本（当前 **1.5354.0**）。`cowork-mdm schema list` 和 `schema show <key>` 把它暴露给你；配置编码器对着它校验；模板库从它取允许值。上游 bump 版本 → 重新抽 → 重新钉 → 重新发版。没有手工维护的键列表会和现实慢慢漂移。

### 2 · 模板是你拥有的 YAML 源，不是不透明的脚手架

每个内置模板都是手写 YAML，放在 [`internal/profile/templates/`](internal/profile/templates/)。`profile show-template NAME --out overrides.yaml` 把同一份 YAML 原样导到你自己的配置仓库里 —— CLI 内嵌的那份和你编辑的这份是逐字节相同的，没有版本漂移的空间。你编辑 YAML，我们编码出 `.mobileconfig` / plist /（即将支持的）`.reg`。`--template` 与 `--from` 在单次调用里刻意互斥：脚手架是起点，不是可下发成品。

### 3 · `validate` 和 `lint` 是两道不同的门，两道都得过

`profile validate` 只做 schema 校验 —— 它把 `REPLACE_WITH_YOUR_API_KEY` 当作合法字符串接受，因为 schema 只说了 `inferenceGatewayApiKey: string`。每个企业模板都在每一个"下发前必须填"的位置留了 `REPLACE_*` 占位符。`profile lint` 扫描成品里残留的这些占位符，发现任何一个就以非零退出。推 YAML 前先 validate 抓 schema bug，把 mobileconfig 交给 Jamf 前跑 lint 作为最后一道门。跳过任何一道，你都会下发一份单看其中一项"通过"的坏配置。

### 4 · MDM 负责配置层，Script payload 负责剩下的

mobileconfig 通道 —— Apple 的 managed-preferences 机制 —— 承载 **LLM 配置**、**独立 MCP 服务**、**出口**、**遥测**、**沙箱**策略。它**无法**承载技能、斜杠命令、hooks、插件内置 MCP。这些住在 `/Library/Application Support/Claude/org-plugins/`，由 `cowork-mdm marketplace add <repo>` 对一个 Claude-Code 格式的插件市场进行下发。企业部署是两波推送：**Wave 1** —— Custom Settings Payload (mobileconfig)。**Wave 2** —— Shell Script Payload (`marketplace add` + `marketplace update`)。两波都针对同一设备组、同一节拍。[为什么这是硬约束而不是 workaround 的逆向证据](docs/research/skills-plugins-mdm.md)。

## 命令参考

按使用意图分组。所有子命令都支持 `--json` 输出机器可读结果。

**编写配置**

```bash
cowork-mdm profile templates                           # 列出内置模板 (9 个)
cowork-mdm profile show-template NAME [--out FILE]     # 导出模板 YAML 源
cowork-mdm profile new --from overrides.yaml --out out.mobileconfig
cowork-mdm profile lint out.mobileconfig               # 标记 REPLACE_* 占位符残留
cowork-mdm profile validate out.mobileconfig           # schema 校验
```

**查询 schema**

```bash
cowork-mdm schema list                          # 全部 51 个键
cowork-mdm schema show inferenceProvider        # 描述、示例值、允许值
cowork-mdm paths show [--os darwin|windows]     # 各平台的读取路径
```

**应用 + 验证**

```bash
cowork-mdm profile apply company.mobileconfig --dry-run    # 预览，不写入
cowork-mdm profile status                                   # 当前活跃的配置
cowork-mdm doctor [--fix]                                   # 诊断异常安装
```

**管理组织插件 (macOS)**

```bash
cowork-mdm marketplace add https://github.com/<org>/claude-org-plugins
cowork-mdm marketplace update
cowork-mdm plugin list
cowork-mdm plugin prune
```

规格文档见 [`specs/`](specs/)；v0.2 完整任务拆解见 [`docs/execution/TASKS.md`](docs/execution/TASKS.md)。

## Claude Code 插件

v0.3 同时发布了 Claude Code 插件层，让 Agent 在你 session 里直接驱动 CLI。

- **Skills (5)**：`cowork-mdm`、`mdm-profile-authoring`、`mdm-profile-deploy`、`mdm-plugins`、`mdm-doctor`。
- **Slash commands (4)**：`/deploy`、`/new-profile`、`/doctor`、`/refresh-plugins`。
- **不含新逻辑。** 每个 skill 都只是 shell 出 `PATH` 上的 `cowork-mdm` CLI。

安装：

```
/plugin marketplace add https://github.com/krislavten/cowork-mdm
/plugin install cowork-mdm@cowork-mdm
```

完整接口见 [`specs/claude-plugin.md`](specs/claude-plugin.md)。

## 下发流水

```
         ┌─────────────────────────────────────┐
         │  Claude Desktop 1.5354.0 (Electron) │
         │  内嵌 zod schema：51 个键           │
         └────────────────┬────────────────────┘
                          │ 抽取 + 钉死到 internal/schema/schema.json
                          ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  cowork-mdm CLI (Go 1.23，单一静态二进制)                    │
  │    schema ─────→ list / show / paths                         │
  │    profile ────→ templates / show-template / new             │
  │                  lint / validate / apply --dry-run / status  │
  │    marketplace → add / update / remove                       │
  │    plugin ─────→ list / prune                                │
  │    doctor ─────→ 主机级诊断 (macOS 下 9 项)                  │
  └─────────┬──────────────────────────────────┬─────────────────┘
            ▼                                  ▼
  ┌──────────────────────┐          ┌──────────────────────────┐
  │  .mobileconfig       │          │  org-plugins/ 目录       │
  │  (Custom Settings)   │          │  (Shell Script payload)  │
  │  LLM · MCP · 出口    │          │  技能 · 斜杠命令         │
  │  遥测 · 沙箱         │          │  插件内置 MCP            │
  └─────────┬────────────┘          └─────────┬────────────────┘
            │                                 │
            └───────────┬─────────────────────┘
                        ▼
           ┌──────────────────────────────────┐
           │  Jamf Pro · Intune · Kandji      │
           │  两波推送到员工 Mac              │
           └──────────────────────────────────┘
```

## 贡献

开发规范见 [AGENTS.md](AGENTS.md)。规格见 [`specs/`](specs/)。欢迎 Issue 和 PR。

## 维护者说明

### 发版

发版由 tag 触发。推送 `v*` tag，`.github/workflows/release.yml` 会调用 GoReleaser。

发版任务同时推到两处：

1. **GitHub Releases** 本仓库 —— 用默认的 `GITHUB_TOKEN`。
2. **Homebrew tap** `krislavten/homebrew-tap` —— 需要本仓库配置 `HOMEBREW_TAP_GITHUB_TOKEN` secret，PAT 对 tap 仓库需要 **contents:write**。没配置时只有 brew formula 推送会失败；GitHub Release + 二进制 + 校验和照常。

一次性配置 secret：

```bash
gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo krislavten/cowork-mdm
# 提示时粘贴 PAT
```

## License

MIT —— 见 [LICENSE](LICENSE)。
