# cowork-mdm

[English](README.md) · **中文**

> 面向企业集群的 Claude Desktop 下发工具链 —— 覆盖 Bedrock / Vertex / Foundry / 第三方及自托管 LLM 网关的 MDM 配置下发、组织插件分发、主机诊断。

**状态**：**v0.3 —— CLI + Claude Code 插件层。** CLI 负责生成 MDM 配置 (`.mobileconfig` / plist)、管理组织插件市场、诊断异常主机。Claude Code 插件层（技能 + 斜杠命令）让 Agent 可以代你驱动 CLI。两者同步发版。

**与 Anthropic 无官方关联。** 本项目基于对公开 Claude Desktop 应用的逆向工程，独立开发维护。

## 为什么需要 cowork-mdm

**Claude Desktop 支持第三方 Anthropic-兼容 LLM provider。** 企业会基于成本、数据驻留、合规、模型偏好等因素选择不同 provider —— Claude Desktop 通过 `inferenceProvider=gateway` 或专用的 Bedrock / Vertex / Foundry 键全都接得上。这涵盖了云厂商路径 (AWS Bedrock / Google Vertex / Azure AI Foundry)、自托管的 vLLM / SGLang 配合 Anthropic-兼容适配层，以及各类第三方网关服务 (DeepSeek / Zhipu GLM / MiniMax / Mistral API 等)。`cowork-mdm` 让 IT 把任意选择下发到整个集群。

**但部署配置并不简单。** 网关 URL + 鉴权方式 + 托管 MCP 服务 + 出口白名单 + 自动更新策略 + 遥测策略 + 沙箱约束 —— Claude Desktop 要读 51 个 managed-preferences 键，其中 Anthropic 公开企业文档只覆盖 8 个。终端用户没办法自助，IT 必须通过 Jamf / Microsoft Intune / Kandji 按集群规模批量下发。`cowork-mdm` 把完整 schema（从应用内嵌 zod 定义提取，当前锁定在 Claude.app 1.5354.0）暴露出来，生成正确的 MDM 配置，提供主机级诊断，让 IT 不需要自己拆 Electron bundle。

**MDM 负责配置层，Script payload 负责其余部分。** LLM 凭据、MCP 服务、出口策略、遥测、沙箱策略全都进 mobileconfig。公司技能、斜杠命令、插件内置的 MCP 放在 `/Library/Application Support/Claude/org-plugins/`，需要配套的 Script payload 调用 `cowork-mdm marketplace add` 来下发。这种混合下发是必须的 —— 反向工程证据见 [docs/research/skills-plugins-mdm.md](docs/research/skills-plugins-mdm.md)。

## 快速开始

```bash
brew install krislavten/tap/cowork-mdm

# 企业 gateway 部署的标准路径：
cowork-mdm profile show-template enterprise-cn-full --out overrides.yaml
$EDITOR overrides.yaml                           # 填写 REPLACE_* 占位符
cowork-mdm profile new --from overrides.yaml \
  --payload-identifier-prefix com.acme.it \
  --out company.mobileconfig
cowork-mdm profile lint company.mobileconfig    # 下发前占位符体检
cowork-mdm profile validate company.mobileconfig
# 接下来通过你公司的 MDM 下发 company.mobileconfig —— 完整步骤见 cookbook。
```

Bedrock / Vertex / Foundry 部署同理，把模板名换成 `bedrock-basic` /
`vertex` / `foundry`，填入 `{{ACCOUNT}}` / region / 模型 ID 占位符即可，
下游流水线完全一致。

## 企业部署手册

**完整手册见 [docs/deployment-cn.md](docs/deployment-cn.md)** —— 八节 gateway 模式端到端流程：

1. 前置准备 2. 选择 LLM provider 3. 生成配置 4. 校验 + lint 5. 通过 Jamf / Intune / Kandji 下发（含插件的 Script payload） 6. 员工机验证 7. 常见失败场景 8. 后续更新。

使用 Bedrock / Vertex / Foundry 的部署同样适用，起点是 [`specs/profile.md`](specs/profile.md) 和内置的 `bedrock-basic` / `vertex` / `foundry` 模板。

## 命令参考

按使用意图分组。所有子命令都支持 `--json` 输出机器可读结果。

**编写配置**
```bash
cowork-mdm profile templates                           # 列出内置模板
cowork-mdm profile show-template NAME [--out FILE]     # 导出模板 YAML 源
cowork-mdm profile new --from overrides.yaml --out out.mobileconfig
cowork-mdm profile lint out.mobileconfig               # 标记 REPLACE_* 残留占位符
cowork-mdm profile validate out.mobileconfig           # schema 校验
```
`--template` 和 `--from` 互斥。`lint` 和 `validate` 互补：`validate` 只做 schema 检查，`lint` 抓脚手架里没填完的占位符。

**查询 schema**
```bash
cowork-mdm schema list                          # 全部 51 个键 (名称、类型、作用域、appMin)
cowork-mdm schema show inferenceProvider        # 描述、示例值、允许值
cowork-mdm paths show [--os darwin|windows]     # 各平台的读取路径
```

**应用 + 验证**
```bash
cowork-mdm profile apply company.mobileconfig --dry-run    # 预览，不写入
cowork-mdm profile status                                   # 当前主机的活跃配置
cowork-mdm doctor [--fix]                                   # 诊断异常安装
```

**管理组织插件 (macOS)**
```bash
cowork-mdm marketplace add https://github.com/<org>/claude-org-plugins
cowork-mdm marketplace update
cowork-mdm plugin list
cowork-mdm plugin prune
```

规格文档与任务拆解：[`specs/`](specs/) + [`docs/execution/TASKS.md`](docs/execution/TASKS.md)。

## Claude Code 插件

v0.3 同时发布了 Claude Code 插件层，让 Agent 能代你驱动 CLI —— 5 个技能 + 4 个斜杠命令，覆盖配置编写、下发、插件管理、诊断。**不引入任何新逻辑**，依赖 `PATH` 上已有的 CLI。完整接口见 [`specs/claude-plugin.md`](specs/claude-plugin.md)。

在 Claude Code 中安装：
```
/plugin marketplace add https://github.com/krislavten/cowork-mdm
/plugin install cowork-mdm@cowork-mdm
```

## 贡献

开发规范见 [AGENTS.md](AGENTS.md)。欢迎 Issue 和 PR。

## 维护者说明

### 发版

发版由 tag 触发。推送一个 `v*` tag，`.github/workflows/release.yml` 会调用 GoReleaser。

发版任务会同时推到两个地方：
1. **GitHub Releases** 本仓库 —— 使用默认的 `GITHUB_TOKEN`。
2. **Homebrew tap** 位于 `krislavten/homebrew-tap` —— 需要本仓库配置 `HOMEBREW_TAP_GITHUB_TOKEN` secret，必须是 PAT（经典或 fine-grained），对 `krislavten/homebrew-tap` 具有 **contents:write** 权限。没配置时 brew formula 推送会失败，但其余步骤（GitHub Release + 二进制 + 校验和）仍然成功。

一次性配置 secret：
```bash
gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo krislavten/cowork-mdm
# 提示时粘贴 PAT
```

## License

MIT —— 见 [LICENSE](LICENSE)。
