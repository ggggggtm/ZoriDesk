# ZoriDesk 当前状态

更新日期：2026-10-27。本文只记录 ZoriDesk 的事实，不把上游验收记录当作本项目复验结果。

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
| 运行时代码 | Q05 分支按授权修复 Loopback 取消/清理，并将暂停恢复改为带编号的有序管道；尚未合入 main |
| 构建、测试、端到端验证 | 新管道实现的本地普通/race 与重复回归通过；Linux CI 普通/race 各 55 个事件通过且无跳过，见下方运行链接。全仓、Swift 应用构建及 E2E 未验证 |
| 应用与服务内部名称 | 仍有 Loom / loomd 等既有标识，尚未完成品牌迁移 |
| 本地开发与提交环境 | 已建立独立 checkout；SSH 认证到正确所有者，仓库级个人提交身份已配置；Q00 记录已真实推送并核对远端 SHA |
| Docker / Kubernetes 后端 | 后续规划，不能写成已交付能力 |
| 发布版本 | 尚未发布 ZoriDesk 版本 |

## 能力边界

继承的代码包含 macOS 工作台、Go 服务、Harness 适配、任务治理、本地事件记录、凭证管理和沙箱接口。源码存在不等于当前环境可用；逐项运行结果应追加到下面的验证记录。

当前 Loopback 使用宿主机进程，不能作为运行任意不可信代码的安全隔离。容器后端也不能仅凭使用 Docker 就宣称生产级强隔离。

不要把改名、导入源码或上游测试结果描述为本项目独立实现。独立成果由真实差异、测试证据和提交记录共同证明。

## 下一步

1. Q00、Q03 经复核并按维护者明确授权合入 main，状态 done；不重复初始化或创建模板。此授权仅针对这两个任务，不改变以后定时维护的分支审查规则。
2. Q05 首个有界切片位于 `codex/q05-sandbox-verification`：可执行验证脚本、测试结果校验器和最小 CI。2026-10-27 获准提前修复阻塞 CI 的取消/子进程回收问题；仅此 Q10 切片例外，不扩展到其他运行改动。Q05 验收合入后才进入 Q09。
3. Q04 随代码和测试同步补充真实路径与关键函数，不阻塞 Q05/Q09；品牌与兼容性任务后置，原有验收、安全边界和分支审查规则保持不变。
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
| 真实推送 | Q00 初始提交 `1ee63aa4309fb16ddc4a45e418f2a9dc27828990`、补齐依赖记录 `0e9257b302714bb47d24d13c97f18630fc2dfbdf` 已推送同名分支，远端 SHA 已核对；初次推送时未合并，后续授权审查与合并见下文 |
| Go | 已安装 1.26.2；go.mod 声明 1.25.5，未下载新工具链 |
| Swift | 6.2.4；包要求 tools 6.0、macOS 14，清单解析通过；不等于整个应用可编译 |
| Xcode | 当前选择 Command Line Tools，`xcodebuild -version` 不可用；现有构建脚本使用 SwiftPM，不能据此断言一定需要完整 Xcode，也不能宣称应用构建已通过 |
| Node / npm | 20.20.2 / 10.8.2；来自用户版本管理器，后台任务需确认 PATH，不能假定服务进程继承相同环境 |
| Pi / gh | 核验时 PATH 未找到；不能据此断言整台机器未安装。Git 已有 SSH 通路，不依赖 gh；Pi 是否为特定功能必需留待 Q11 核验 |
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

## Q03 贡献流程与模板：2026-10-13

基准：`47a0717e57ca9704dbe37ca76e154c30d1e6c81c`（main）。原始分支：[codex/q03-contribution-guide](https://github.com/ggggggtm/ZoriDesk/tree/codex/q03-contribution-guide)，提交 [b871df3](https://github.com/ggggggtm/ZoriDesk/commit/b871df3c084f4562f2edc880f20b076296f2fbc9)。最初独立开发，未叠加 Q00；后经授权审查合入 main，状态 `done`。

- 新增贡献指南、缺陷/功能请求模板、PR 模板和 README 导航，明确复现、验证、兼容、脱敏与本仓库目标。
- 公共 GitHub API 只读核验：Issues 和 Discussions 未启用，无开放 PR。模板作为源码交付；需合入默认分支且启用相应功能后才能验证网页呈现。本轮未开启功能、创建 PR 或修改权限。
- 静态检查通过：Ruby `YAML.safe_load` 解析两份 Issue 模板并校验 name/about/title；Node 检查 7 份 Markdown、20 个相对文件链接、必需栏目与代码围栏，并做限定敏感模式扫描；`git diff --cached --check` 检查完整暂存差异。另人工复核目标仓库、来源及权限边界；模式扫描不是完整安全审计。本轮不修改运行代码，不重复执行应用构建或真实模型测试。
- Q03 首次交付引用 Q00 提交 [0e9257b](https://github.com/ggggggtm/ZoriDesk/commit/0e9257b302714bb47d24d13c97f18630fc2dfbdf) 的 11 项测试证据，当时未重跑；随后授权合并审查确实重新执行，见下节。保留这一区别，避免把引用证据当成本轮测试。
- 合并保留了 Q00 环境与失败/复验记录、Q03 模板交付记录；没有整份覆盖任一分支。网页模板交互未验证，Issues 仍不自动启用，也没有 CI 运行证据。

## Q00 / Q03 授权合并审查：2026-10-13

维护者明确要求重新 review，确认无问题后合并 main。审查范围为 Q00 的 `1ee63aa`、`0e9257b` 与 Q03 的 `b871df3`，不含后续功能开发。

- 重新核对专用 checkout、干净工作树、origin、个人提交身份和 SSH 所有者；仅使用已验证的 Git 通道，不使用其他账号的连接器写入。
- 使用上述离线命令重新执行 `go mod verify`、sandbox 包测试及 Swift 清单解析：全部通过，11 项测试无跳过；go.mod/go.sum SHA-256 与原记录一致，没有下载、升级或修改运行代码。
- 再次检查两份 Issue 模板 YAML、贡献/反馈/审查栏目、文档链接和完整差异；修正“PATH 未找到等于未安装”的不准确措辞及 README 已过期的验证状态。
- 解决 STATUS.md 的合并冲突，保留两分支全部有效证据；Q00、Q03 按本次授权合入 main 后完成。Q05 仅解除前置依赖，未执行其任务。
- 未覆盖：全仓测试、race、应用构建/安装、真实模型 E2E、网页模板交互；Q00/Q03 的完成不代表这些能力已通过。

## Q05 首个验证切片：2026-10-24

基准：main `b582039c51e8a145802f1295ac2704b10b38abf4`。分支：[codex/q05-sandbox-verification](https://github.com/ggggggtm/ZoriDesk/tree/codex/q05-sandbox-verification)。此处日期是实际执行日期，不回填计划触发日期。

- 新增 `scripts/verify-sandbox.sh`、`internal/testreport` 与只读权限的最小 GitHub Actions 工作流；同步 [VERIFICATION.md](VERIFICATION.md) 解释输入、执行顺序、失败路径和覆盖边界。没有修改 sandbox 的运行逻辑、依赖锁文件或继承的上游历史。
- 先新增结果校验回归，实际获得 `undefined: validate` 编译失败，再实现校验器。22 个表驱动子用例覆盖成功、多包、并行事件、skip/fail、无测试、缺包、未结束测试、重复事件和损坏 JSON；输出中的 SKIP 文字不冒充结构化 skip 事件。
- 本地 macOS/arm64、Go 1.26.2：`bash scripts/verify-sandbox.sh` 与 `bash scripts/verify-sandbox.sh --race` 均通过；目标格式检查、`go mod verify`、sandbox 包编译、两包 vet 通过。每轮共 34 个成功 test/subtest 事件，具体为 sandbox 11 个顶层测试 + 校验器 1 个顶层测试及 22 个子测试；无跳过，不把子测试计数称为 34 个顶层测试。
- `bash -n`、不支持参数返回退出码 2、工作流 YAML 解析/权限检查、`git diff --check` 与 go.mod/go.sum 无变化检查通过。整个验证入口离线使用缓存，未下载工具/依赖、调用模型、启动应用或安装服务。
- 工作流使用固定 checkout SHA，runner 现有 Go、锁定公开依赖和相同 race 入口。尚无本切片的 GitHub CI 成功证据，不自动开启 Actions，不称 CI 已通过；推送后单独核验运行结果。
- Q05 仍 `in_progress`，本切片供审查；Q04 仅补验证路径，完整架构未完成。未运行全仓测试、Swift 应用构建、真实模型 E2E 或 Docker。Q09 等待 Q05 验收合入，不因本地测试通过提前执行。

## Q05 CI 工具链修复：2026-10-27

- 已确认 [首次运行 40056339209](https://github.com/ggggggtm/ZoriDesk/actions/runs/40056339209) 在依赖下载阶段失败：runner 的 Go 1.24.13 低于 go.mod 要求的 1.25.5，且 `GOTOOLCHAIN=local` 禁止隐式升级；race 步骤被跳过，不是测试通过或业务断言失败。
- 用户明确授权 GitHub CI 下载对应 Go 工具链。原分支补上固定 SHA 的 `actions/setup-go`，先从 go.mod 准备 Go，再下载锁定依赖；不改本机工具链、依赖版本、权限或测试覆盖。
- 修复提交 `129da7910e3189a7fc76526504ad89bfc2a6ee0e` 已推送原分支；配置顺序、固定 SHA、只读权限检查及本地 race 基线通过，依赖锁文件未变。
- [运行 40497053197](https://github.com/ggggggtm/ZoriDesk/actions/runs/40497053197) 已在 Linux/amd64 成功下载并使用 Go 1.25.5，依赖下载成功，实际执行了 race 基线；最终仍失败，不能标 CI 通过。唯一失败用例为 `TestLoopbackCancelTerminatesLongCommand`：`loopback_test.go:98: cancel did not terminate the command`，5 秒内未收到取消后的执行结果。
- 源码核验：`Exec` 使用 `sh -c` 并收集输出，`Cancel` 只对主进程调用 `Process.Kill`。未清理子进程及其继承的管道是与日志一致的原因推断，尚未逐进程观测；测试自身还以固定 300ms 等待进程启动，后续需排除启动竞态。不能用放宽超时或跳过测试掩盖问题。
- 此次工具链提交没有改进程运行逻辑；随后维护者明确授权在原分支提前修复取消/子进程回收，并要求真实 CI 通过、不跳过测试、不合并 main。后续记录见下节；全仓、Swift 应用与真实 E2E 仍未验证。

## Q05 阻塞修复首轮记录：取消与同组子进程清理（2026-10-27）

维护者明确授权提前修复本切片，原分支继续开发，不合并 main。以下记录被后续有序管道方案替代的失败候选，保留失败证据而非当前完成声明。修复前先将取消测试改为等待真实子进程就绪、启动长于断言期限的复合命令；旧实现本地也复现“取消后仍未返回”，排除了只等待固定 300ms 的不确定性。

- `LoopbackBackend.Exec` 使用独立进程组和监督 shell。取消由仍存活的监督进程终止自身进程组，Go 只操作已持有的进程句柄，不用可能已复用的组编号清理。正常结束也清理遗留后台进程，并保留子命令退出码；清理异常不能伪装成功。
- 暂停/恢复尝试通过确认管道处理“恢复信号先到、随后才暂停”的竞态，但最终复验仍有间歇失败，不能认定问题已解决。同实例并发执行拒绝覆盖归属，`Destroy` 等待执行清理后移除目录，上下文取消不等待启动阶段持有的全局锁。
- 新增受控子进程就绪/状态检查，覆盖显式取消、context 取消/超时、暂停后取消、正常退出 0/7 的后台进程清理、重复取消/销毁、实例间隔离、启动取消、连续暂停恢复和恢复后退出码。没有新增 skip、降低 CI 检查要求或修改依赖锁文件。
- 独立审查发现并修正初版的暂停回归、清理状态校验遗漏；额外只读分析确认信号竞态及确认协议。最终本地检查与远端结果在下列记录中区分，不将本地成功当成 CI 成功。
- 本地 macOS/arm64、Go 1.26.2：最终 `bash scripts/verify-sandbox.sh --race` 通过，49 个 test/subtest 事件、无跳过；加入最后一个退出码测试前，Loopback race 回归连续 10 轮通过。调用方影响检查 `TestSandboxWire*` 3 项、`TestRedSB2*` 4 项通过，均无跳过。
- **最终普通测试失败**：`GOPROXY=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go test ./internal/sandbox ./internal/testreport -count=1 -timeout=90s` 在 `TestLoopbackPauseResumeStopsWorkload` 的快速暂停/恢复循环失败，`loopback_test.go:402: loopback: supervisor did not acknowledge resume`；sandbox 包失败，testreport 包通过。race 成功不能覆盖此反例。
- 按开发流程的连续修复失败停止规则，保留本地候选和证据、请求维护者决定下一步；不跳过测试、不提交或推送失败实现。本轮没有触发新 CI，远端仍为 `d9da3dc`，Q05/Q10 未验收，Q09 不开展。源码路径和候选机制见 [VERIFICATION.md](VERIFICATION.md#本次取消修复从执行到清理)。

边界：Loopback 仍为可信测试命令的宿主机执行后端；主动逃离进程组不在保证内。直接子进程由 Go 等待，孤儿后代的僵尸回收由操作系统负责；“不再运行”不等于已由 Go 回收全部后代。启动取消循环是竞态回归，不宣称精确覆盖启动中的每一条调度路径。未验证全仓、Swift 应用、真实模型 E2E 或 Docker；运行期输出内存约束仍属于 Q09。

## Q05 确认协议重设计：2026-10-27

维护者在首轮停止后明确允许继续重设计、补回归、通过普通/race 后推送原分支，再验证真实 CI；仍不合并 main。基准仍为 `d9da3dc`，没有改写远端历史。

- 原信号确认方案的普通重复回归 20 轮实际失败 8 轮。静态分析发现：确认发在 `USR1` 处理器内部，下一次请求可能先于处理器退出；普通信号不是可靠请求队列。此处不声称已通过系统信号跟踪证明某个 shell 的具体丢信号位置。
- 删除重复 `USR1` 控制，改为监督进程主循环读取 `pause <序号>` 与包装进程发出的 `exit <退出码>`；完整短记录通过同一管道排队。恢复后返回 `resumed <序号>`，只有匹配当前待确认编号才能完成；错误/未来编号拒绝，旧编号不能确认新请求。恢复中断保留待确认状态以便重试。
- 实际用户命令关闭全部控制描述符。取消仍通过存活监督进程自清理所属进程组，不在 Go 父进程中用旧 PGID 发信号；子命令正常退出码仍保留。源码、协议和限制见 [VERIFICATION.md](VERIFICATION.md#本次取消修复从执行到清理)。
- 新增协议格式/序号校验、一次排入两条暂停消息并验证确认顺序、恢复失败后重试，以及暂停期间取消/超时和恢复取消并发的回归；保留原失败测试，没有放宽确认时限或跳过断言。
- 本地最终普通/race 基线均通过，每轮 55 个 test/subtest 成功事件、无跳过；包含格式、依赖校验、编译及 vet。Loopback 全部回归普通模式连续 20 轮通过、race 连续 10 轮通过；后补三种取消交错场景后，完整取消用例另连续 10 轮普通测试通过，源码未再变更。调用方 `TestSandboxWire*` 3 项及 `TestRedSB2*` 4 项通过。
- CI 增加普通基线步骤，保留 race、结构化零跳过校验、固定工具版本、只读权限和依赖锁不变检查。工作流 YAML/步骤检查及 `git diff --check` 通过，go.mod/go.sum 未变。本机不安装工具、不调用模型或启动真实服务。
- 独立只读审查未发现当前协议的阻断问题。修复已提交并推送原分支：[335f6f5](https://github.com/ggggggtm/ZoriDesk/commit/335f6f54562302927f39a6fab80fab0d90b9e6a6)。远端 SHA 与本地一致；main 仍为 `b582039c51e8a145802f1295ac2704b10b38abf4`，没有合并。
- **实际 CI 成功**：[运行 40511027334](https://github.com/ggggggtm/ZoriDesk/actions/runs/40511027334)，作业 `122212667649`，Linux/amd64、Go 1.25.5，总时长 1m21s。已查看成功状态和完整日志：普通模式、race 模式各输出 `Verified 55 passing test/subtest records across 2 packages; no skips.`，依赖锁检查通过。不是仅确认工具链安装成功。
- Q05 本切片状态 `review`，未合入 main 不记 done；Q10 只交付本次授权的取消/清理切片，其余验收仍待后续。Q09 等待 Q05 经用户审查合入，不自动推进。此后的纯状态文档提交会触发新 CI，其对应状态必须另外核验，不能借用上述代码提交结果。

## 状态维护约定

- 实现写在分支，不表示已进入 main；待审查和已合并必须区分。
- 代码验收至少包含目标行为、异常路径和回归检查。未具备运行条件时标记 blocked 或 review，不能标记 done。
- 发现既有能力已经满足需求时，先给证据，再缩小任务或提出跳过建议，不重复实现。
- 不在本文件放个人目录、账号凭据、内部工作材料或完整敏感日志。
