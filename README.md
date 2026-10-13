# ZoriDesk

**本地优先的多 Agent 协作与执行工作台。**

Local-first multi-agent workspace, built with Go and SwiftUI.

ZoriDesk 将 Agent 对话、任务协作、执行审批和结果追踪放在同一个桌面工作台中。面向开发者，逐步探索更清晰的运行工具接入、更可靠的任务执行和可切换的沙箱后端。

> **项目状态：初始化 / 持续建设。** 本次初始化调整项目介绍与演进方向，尚未改变运行逻辑，也没有重新验证构建和端到端运行。应用名称、后台进程 `loomd`、模块路径与安装脚本暂时沿用既有实现，后续分阶段统一。

[当前状态](STATUS.md) · [详细任务队列](TASKS.md) · [维护规则](AGENTS.md) · [贡献指南](CONTRIBUTING.md) · [改进路线](#改进路线) · [源码导航](#源码导航) · [架构文档](docs/ARCHITECTURE.md) · [开发指南](docs/DEVELOPMENT.md) · [许可证](LICENSE)

## 解决什么问题

使用多个 Agent 工具完成开发任务时，对话、执行环境、权限和结果容易分散。ZoriDesk 希望让这些环节保持可理解、可干预和可追踪：

1. 在桌面端开始对话，选择运行工具和模型服务。
2. 将明确目标组织为 Mission（任务），审核 Agent 团队与执行配置。
3. 在受控权限下执行，查看各 Agent 的状态与输出。
4. 检查结果与执行记录，在失败或阻塞时提供指导并继续。

普通对话不会自动创建团队；模型输出是建议，不是执行授权。

## 已有代码基础

以下能力继承自上游实现，不代表 ZoriDesk 新增，也不代表已在当前环境验证：

| 模块 | 作用 |
| --- | --- |
| macOS 工作台 | SwiftUI 对话、Mission 与运行工具配置界面 |
| Go 后台服务 | 本地接口、任务协调、权限判断和执行管理 |
| Harness 适配 | 对接 Codex、Claude Code、OpenCode、Pi 等 Agent 工具 |
| 任务与验收 | 团队任务组织、执行状态、结果证据和人工干预 |
| 本地状态 | SQLite 事件记录，以及可重建的查询视图 |
| 凭证管理 | 凭证存储、短期使用授权和本地通信边界 |
| 沙箱接口 | 创建、执行、取消、暂停、恢复、销毁的后端抽象 |

**沙箱边界：** 当前 Loopback 后端通过本机进程执行，不是生产级隔离环境。Docker 和远端 Kubernetes 沙箱是计划方向，尚未实现为本项目新增能力。不要将现有执行器用于任意不可信代码。

## 改进路线

按小范围、可验证的变化逐步推进，不以提交数量作为目标。

- [x] 建立 ZoriDesk 仓库，保留既有代码与提交历史。
- [x] 整理项目定位、源码导航与能力边界。
- [ ] 统一项目品牌：应用名、服务标识、构建与安装配置，明确数据迁移策略。
- [ ] 完善开发文档和无需真实模型账号的基础检查流程。
- [ ] 解耦首次启动与特定运行工具的依赖，改善配置与错误提示。
- [ ] 补充输出大小限制、取消和资源回收相关回归测试。
- [ ] 实现 Docker 执行后端，明确资源、挂载、网络与凭证边界。
- [ ] 在本地执行闭环稳定后，评估远端 Kubernetes 沙箱接入。

每项计划在实现和验证后再标记完成；性能数字、兼容性和安全性结论以实际测试为准。

## 源码导航

| 路径 | 内容 |
| --- | --- |
| `apps/macos/` | 桌面应用与 SwiftUI 界面 |
| `cmd/loomd/` | 后台服务入口与组件装配 |
| `internal/app/` | 任务和配置协调 |
| `internal/harnessgateway/` | Agent 工具会话与生命周期管理 |
| `internal/runtime/` | 运行工具发现与适配 |
| `internal/execution/` | 工具执行与策略检查 |
| `internal/sandbox/` | 沙箱接口与实验后端 |
| `internal/credentials/` | 凭证管理 |
| `internal/journal/` | 事件持久化 |
| `internal/projection/` | 状态查询视图 |

建议先看服务入口与组件装配，再沿一次任务进入协调、执行与结果记录链路。

## 开发起点

当前 macOS 应用面向 macOS 14+、Apple silicon；Swift 工具链要求见 `apps/macos/Package.swift`，Go 版本要求见 `go.mod`（当前声明 Go 1.25.5）。运行工具及其依赖需要单独配置。

```sh
git clone https://github.com/ggggggtm/ZoriDesk.git
cd ZoriDesk
```

修改前阅读 [AGENTS.md](AGENTS.md) 与相关代码；历史架构和开发文档可辅助理解。`docs/CURRENT.md`、`CHANGELOG.md` 和 `.loom-evidence/` 中的既有验收记录属于上游历史，不代表本项目已经复验或发布。

本轮未执行构建、测试、安装或真实模型调用。后续验证将区分源码检查、本地运行与使用真实账号的端到端测试；未经明确授权不消耗模型额度。

## 来源与许可

ZoriDesk 基于 [Loom](https://github.com/tttboy123/loom) 演进，初始化基线为 `dae84547bba0b9a429e5ad5dfd12b85505f94bc4`。保留上游提交历史及必要版权说明，使用 [Apache-2.0](LICENSE) 许可证。

本 README 由 ZoriDesk 维护者于 2026-10-13 为项目初始化改写；后续独立改动以本仓库的实际提交和验证记录为准。
