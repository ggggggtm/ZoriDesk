# ZoriDesk 当前状态

更新日期：2026-10-13。本文只记录 ZoriDesk 的事实，不把上游验收记录当作本项目复验结果。

## 项目身份与阅读顺序

- 维护仓库：[ggggggtm/ZoriDesk](https://github.com/ggggggtm/ZoriDesk)，默认分支 main。
- 产品定位：本地优先的多 Agent 协作与执行工作台，Go 后台 + macOS SwiftUI 客户端。
- 上游基础：tttboy123/loom，初始化基线 dae84547bba0b9a429e5ad5dfd12b85505f94bc4；保留 Apache-2.0、必要版权与提交历史。
- 先读本文，再读 [TASKS.md](TASKS.md) 选择任务，按 [AGENTS.md](AGENTS.md) 执行。
- docs/CURRENT.md、PROGRESS.md、CHANGELOG.md、.loom-evidence/ 中继承的进度、工作区、版本和验收记录属于上游历史；不得用于认定 ZoriDesk 的本地目录、开发状态或发布状态。

## 当前已确认

| 事项 | 状态与证据 |
| --- | --- |
| 公开仓库初始化 | 已完成；保留上游代码和历史，使用 ZoriDesk 名称 |
| 项目介绍 | 已发布 README，提交 510fdc956d74e533eb6b43551e1fffc3d10d24ef |
| 运行时代码 | 本项目初始化尚未修改运行逻辑 |
| 构建、测试、端到端验证 | Q00 的 internal/sandbox 包 11 项现有测试、依赖校验与 Swift 清单解析通过；全仓测试、应用构建和 E2E 未验证 |
| 应用与服务内部名称 | 仍有 Loom / loomd 等既有标识，尚未完成品牌迁移 |
| 本地开发与提交环境 | 已建立独立 checkout；SSH 认证到正确所有者，仓库级个人提交身份已配置；Q00 记录已真实推送并核对远端 SHA |
| Docker / Kubernetes 后端 | 后续规划，不能写成已交付能力 |
| 发布版本 | 尚未发布 ZoriDesk 版本 |

## 能力边界

继承的代码包含 macOS 工作台、Go 服务、Harness 适配、任务治理、本地事件记录、凭证管理和沙箱接口。源码存在不等于当前环境可用；逐项运行结果应追加到下面的验证记录。

当前 Loopback 使用宿主机进程，不能作为运行任意不可信代码的安全隔离。容器后端也不能仅凭使用 Docker 就宣称生产级强隔离。

不要把改名、导入源码或上游测试结果描述为本项目独立实现。独立成果由真实差异、测试证据和提交记录共同证明。

## 下一步

1. 审阅 Q00 环境记录及分支，待合并不标记 done。已按授权下载锁定公开 Go 依赖，sandbox 包测试的缓存阻碍已解除；不扩大为依赖升级或其他工具安装授权。
2. 优先 Q03 贡献说明和模板、Q04 架构与用户流程；环境未就绪不妨碍有依据的文档工作。
3. Q05 建立不依赖真实模型账号的验证基线后，再开始应用标识迁移与运行行为修改。
4. 执行前检查实际分支、未提交改动和待审查工作；有未完成任务先收尾，不机械开始新任务。

## 验证记录

| 日期 | 范围 | 结果 | 未覆盖 |
| --- | --- | --- | --- |
| 2026-10-13 | 仓库与 README 初始化 | GitHub 页面可见，README 提交如上 | 构建、单元测试、真实模型调用、安装、端到端运行 |
| 2026-10-13 | Q00 开发与提交通路 | 独立 checkout、正确 SSH 身份、仓库级提交身份、推送 dry-run；最小 Go 接口测试及 Swift 清单解析通过 | 完整 Go 包测试受依赖缓存阻塞；未执行应用构建、安装、真实模型 E2E 或 Docker 容器启动 |
| 2026-10-13 | Q00 授权后依赖补齐与复验 | 锁定公开 Go 依赖下载及校验成功；sandbox 包 11 项现有测试通过；go.mod/go.sum 哈希不变 | 未执行全仓测试、race 检查、应用构建、安装或真实模型 E2E；单包通过不等于后续可靠性任务已完成 |

后续每次记录：任务 ID、基准提交、改动分支/提交链接、环境、实际检查命令、通过/失败/跳过、未覆盖范围及原因。只记录完成的检查，不补写未执行的结果；无需为重复且无变化的检查制造提交。

## Q00 环境核验：2026-10-13

基准提交：`47a0717e57ca9704dbe37ca76e154c30d1e6c81c`。核验分支：[codex/q00-environment-check](https://github.com/ggggggtm/ZoriDesk/tree/codex/q00-environment-check)。本地绝对路径仅保存在私有调度配置，不公开到仓库。

| 检查 | 结果与限制 |
| --- | --- |
| 操作系统 | macOS 15.6.1 / arm64 |
| Git | 2.50.1；从本仓库建立独立副本，初始工作树干净；检查时无开放 PR |
| 身份与远端 | `origin` 为 `git@github.com:ggggggtm/ZoriDesk.git`；SSH 明确认证为 ggggggtm；仅设置仓库级个人姓名/邮箱，没有修改全局身份或凭据 |
| 推送预检查 | `git push --dry-run origin HEAD:refs/heads/codex/q00-environment-check` 通过；dry-run 本身不等于真实推送，需在记录提交后核对远端分支 SHA |
| 真实推送 | Q00 提交 `1ee63aa4309fb16ddc4a45e418f2a9dc27828990` 已推送同名分支，`git ls-remote` 与本地 SHA 一致；没有合并 main |
| Go | 已安装 1.26.2；go.mod 声明 1.25.5，未下载新工具链 |
| Swift | 6.2.4；包要求 tools 6.0、macOS 14，清单解析通过；不等于整个应用可编译 |
| Xcode | 当前选择 Command Line Tools，`xcodebuild -version` 不可用；现有构建脚本使用 SwiftPM，不能据此断言一定需要完整 Xcode，也不能宣称应用构建已通过 |
| Node / npm | 20.20.2 / 10.8.2；来自用户版本管理器，后台任务需确认 PATH，不能假定服务进程继承相同环境 |
| Pi / gh | 当前 PATH 未找到；均未安装。Git 已有 SSH 通路，不依赖 gh；Pi 是否为特定功能必需留待 Q11 核验 |
| Docker | CLI 28.3.3、服务端 28.4.0，当前 Colima context 可连接；只查询状态，未拉取镜像或创建容器 |

实际执行的最小检查：

```sh
# 仓库根目录；禁用下载，只用现有工具链与缓存。
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -mod=readonly ./internal/sandbox -run '^TestRedSB1_SevenOpsRecorded$' -count=1
# 结果：setup failed，缺少 modernc.org/sqlite v1.35.0 缓存；没有执行到目标测试。

GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -mod=readonly internal/sandbox/backend.go internal/sandbox/backend_test.go -run '^TestRedSB1_SevenOpsRecorded$' -count=1
# 结果：通过。仅为这两个文件的接口冒烟测试，不覆盖 sandbox 包其他代码。

swift package --package-path apps/macos dump-package
# 结果：通过。仅验证包清单可解析，不是应用构建或 UI 测试。
```

### 授权后的依赖补齐与包级复验

2026-10-13 获准下载 go.mod/go.sum 锁定的公开 Go 依赖；不升级版本、不安装其他工具、不调用模型。以下命令均在仓库根目录执行：

```sh
GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org go mod download
# 成功；保持现有本地工具链，启用公开校验服务。

GOPROXY=off GOTOOLCHAIN=local go mod verify
# all modules verified

GOPROXY=off GOTOOLCHAIN=local go test -mod=readonly ./internal/sandbox -count=1 -timeout=90s -v
# PASS：11 项现有测试，无跳过；包括接口、Loopback 执行/取消/清理及基于 SQLite 的状态记录与重建。

git diff --exit-code -- go.mod go.sum
# 无差异；下载前后 SHA-256 也一致。
```

- go.mod SHA-256：`c6705b2b06382d4461bb2c17116336033350d18a02f617efa67a88162a0ec09e`
- go.sum SHA-256：`cd0727970dad12df40a570b1332f05d8cadde2cdaaffbfe5e4dc88db5d20d7af`

此前缺失 SQLite 缓存的阻碍已解除。后续 Q05 仍需建立更广的验证基线；这次没有执行全仓测试、race 检查或应用构建，也不代表 Q09/Q10 的边界场景已经覆盖。本轮只补齐 Go 依赖和记录实际验证，没有安装额外工具、启动应用、消耗模型额度或修改运行代码。不要通过删除测试、伪造成功或关闭既有安全钩子绕过后续阻碍。

## 状态维护约定

- 实现写在分支，不表示已进入 main；待审查和已合并必须区分。
- 代码验收至少包含目标行为、异常路径和回归检查。未具备运行条件时标记 blocked 或 review，不能标记 done。
- 发现既有能力已经满足需求时，先给证据，再缩小任务或提出跳过建议，不重复实现。
- 不在本文件放个人目录、账号凭据、内部工作材料或完整敏感日志。
