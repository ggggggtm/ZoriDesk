# 无账号的 Sandbox 与启动验证入口

状态：Q05/Q09/Q10 的有界 Sandbox 验证、Q11 启动回归、Q12诊断验收及Q06首个品牌切片，不代表完整应用通过验收。运行结果见 [STATUS.md](STATUS.md)，任务依赖见 [TASKS.md](TASKS.md)。

## 怎么运行

需要现有 Bash、Go（版本下限见 go.mod）、gofmt 和系统基础工具。race 模式还需要受支持的平台及可用 C 编译工具链。命令可以从任意目录调用；脚本会定位自身所在仓库。

```sh
bash scripts/verify-sandbox.sh
bash scripts/verify-sandbox.sh --race
```

脚本强制 `GOTOOLCHAIN=local`、`GOWORK=off`、`GOFLAGS=-mod=readonly`、`GOPROXY=off`，不下载工具链或依赖、不升级版本、不读取真实模型账号、不启动应用。依赖缺失时失败，不把测试未执行当作通过。首次准备缓存需要另行获准后下载 go.mod/go.sum 锁定的公开 Go 模块；维护者已批准本项目该范围的下载，不代表安装其他工具也获准。

检查顺序：目标包 gofmt（只检查不改写）→ 缓存完整性 → sandbox 包编译 → 两包 go vet → 未缓存的 Go 测试 → JSON 结果校验。失败返回非零。临时 JSON 文件退出时清理，终端输出仍保留；发布日志前应复核脱敏。

## 为什么还要校验测试结果

`go test` 可能在测试全部跳过时仍退出成功；只搜索输出中的 `PASS` 也可能把日志正文误当结果。本入口读取结构化事件，要求每个指定包都有测试执行和最终成功事件，所有已开始测试都有成功结束事件，并拒绝 skip、fail、缺包、损坏 JSON 和空测试集。

- [verify-sandbox.sh](scripts/verify-sandbox.sh)：限定包范围，使用 `pipefail` 保留测试命令的失败状态，不允许通过额外参数或继承 GOFLAGS 静默筛掉测试。
- [testreport/main.go](internal/testreport/main.go)：`validate` 逐条解码 `go test -json`；按包和测试名关联 run/pass，再验证预期包集合。它只校验结果，不运行测试。
- [testreport/main_test.go](internal/testreport/main_test.go)：正常、并行调度、多包、跳过、失败、缺失、重复和损坏输入的回归。日志中的“SKIP”文字不是 skip 事件。

这是可信 Go 测试输出的验收辅助，不是对恶意伪造测试证据的认证系统。成功计数包含子测试，不等同于独立顶层用例数；脚本只接受本次固定包集合。

## 现有 Sandbox 测试实际覆盖什么

- [backend_test.go](internal/sandbox/backend_test.go)：recording backend 验证接口调用记录，是 mock，不是真实容器执行。
- [loopback_test.go](internal/sandbox/loopback_test.go)：在临时目录执行本地 shell，覆盖取消、context 超时、暂停/恢复、同组子进程终止、重复清理、正常结束遗留后台进程及实例间隔离。Loopback 是宿主机进程，不提供强隔离；不等于完整验证环境变量泄漏和任意进程树回收。
- [reconcile_test.go](internal/sandbox/reconcile_test.go)：临时 SQLite journal + fake backend，验证事实记录、失败重放、generation 拒绝、状态重建和清理幂等；不读取真实工作台数据库。
- [output_tail_test.go](internal/sandbox/output_tail_test.go)：固定尾缓冲边界/并发和真实 Loopback 大输出、取消、临时 journal 重放；具体契约见下方 Q09。

未覆盖：全仓编译/测试、Swift 应用构建与 UI、安装/迁移、真实模型 E2E、Docker/Kubernetes、所有产品输出通路的上限及任意进程树取消语义。不要以本入口通过替代这些验收。

## 本次取消修复：从执行到清理

**已于 2026-10-28 经复核按维护者授权合入 main：**旧信号确认方案曾在普通测试失败，当前重新设计为有序管道协议。本地与远端的实际验证结果分别见 STATUS.md，不把测试通过扩大为生产级隔离承诺；Q10 仍只完成本次授权切片。

原来 `Exec` 启动 `sh -c`，`Cancel` 只终止这个 shell；其子进程仍可能运行并持有 stdout/stderr 管道，导致 Go 的 `Wait` 继续等待读取结束。Linux CI 暴露了此问题。回归改为等待子进程写出就绪标记，再发取消，避免靠固定启动延迟碰运气；子进程的自然结束时间大于断言期限，不能靠等命令自行完成过关。

当前链路位于 [loopback.go](internal/sandbox/loopback.go)：

1. `Exec` 为每次执行创建独立进程组，启动固定的监督 shell。它先安装最终取消处理器，再经状态管道报告就绪；包装进程启动用户命令，等待结束后向控制管道写入 `exit <退出码>`。实际用户命令关闭全部控制文件描述符，不能把普通输出混入协议。
2. 父进程只通过 `os.Process` 句柄请求取消。活着的监督进程终止自己的进程组，避免父进程在组长被回收后用旧进程组编号发信号，误伤编号复用后的其他任务。
3. 普通完成也先记录用户命令退出码，再清理同组后台进程。`validateLoopbackCompletion` 检查监督进程的结束方式；清理失败不能仅凭子命令退出码 0 被当成成功。正常命令退出码保留；context 取消/超时无正常状态时返回 124，显式取消无正常状态时返回非零。
4. `Pause` 向控制管道写入 `pause <递增序号>`，监督进程主循环按完整消息顺序处理并暂停自身进程组。`Resume` 唤醒仍存活的监督进程；监督进程继续执行、唤醒全组，再通过独立管道返回 `resumed <同一序号>`。下一次请求即使提前到达也会排在管道中，不再依赖 `USR1` 信号处理器退出时机。请求发送成功不等于所有进程已立刻暂停；暂停生效是异步的，测试通过进程状态和工作进度确认。
5. `parseLoopbackResumeAck` 校验确认格式，`matchLoopbackResumeAck` 只让当前序号完成等待；旧序号不能完成新请求，未来/异常序号明确报错。每个执行串行处理暂停/恢复，恢复失败保留待确认序号以便重试，终止/EOF 不伪装确认成功。取消独立于控制锁，防止暂停或恢复等待挡住清理。
6. 同实例并发 `Exec` 被拒绝，避免覆盖进程归属；`Destroy` 阻止新执行、等待已有执行清理后再移除本实例目录，重复调用可重试。不同实例不会因为一方取消被一起终止。

例如一次暂停编号为 7，恢复必须收到 `resumed 7` 才清除该待确认请求；随后 `pause 8` 即使在监督进程刚写完确认时到达，也会留在管道里等待读取。若命令完成事件先于暂停事件被处理，则执行结束，恢复会收到终止错误；并不保证已结束的命令还能暂停恢复。该协议是进程内生命周期控制，不是可持久化、可跨服务重启恢复的任务队列。

这里的“清理”指受控命令及同组后代不再运行、管道关闭、直接子进程由 Go 等待回收；成为孤儿的后代若暂时为僵尸，由操作系统的收养进程回收，不能宣称 Go 已回收全部后代。主动 `setsid` 逃逸进程组、攻击监督进程等恶意行为不在此宿主机测试后端的保证内。Q05 本身未解决输出缓存增长；下节描述独立 Q09 分支的改动，合并状态以 STATUS 为准。

## Q09：为什么输出再多也不用全部存下

**已于 2026-10-28 经复核按维护者授权合入 main。**当前 `ExecResult` 只有退出码与摘要，没有 stdout/stderr 原文。旧逻辑是收集全部输出，结束后只对最后 64 KiB 算 SHA-256；本次保留这个结果契约，把限制前移到每次写入时。

流程：命令 stdout/stderr → 同一个 `outputTail.Write` → 固定 64 KiB 环形数组 → `command.Wait` 等待输出管道读完 → `outputTail.Digest` → `ExecResult`。写满后覆盖最旧数据，超大单次写入直接保留该块尾部；返回长度仍是原始块长，避免短写让管道读取中断、影响子进程或造成死锁。并发访问由同一把锁保护。

例如先输出 2 MiB 数据再输出 `failure`，只保留“此前数据最后 65529 字节 + failure”，算出的摘要与旧实现完全相同。溢出不是错误，不改变真实退出码；也没有把额外的“已截断”文本塞进摘要。摘要不是全文完整性证明：两个输出只要最后 64 KiB 相同，摘要就相同，这是兼容既有契约的明确限制。

- 关键实现：[output_tail.go](internal/sandbox/output_tail.go) 的 `Write`、`Digest`；[loopback.go](internal/sandbox/loopback.go) 的 `Exec` 将两个输出流接到同一 writer。
- 事件路径：[reconcile.go](internal/sandbox/reconcile.go) 的 `Controller.Exec` 将结果写入 `SandboxExecCompleted`，重复 operationID 从事件记录返回，不重复运行命令；字段和历史数据格式未改。
- 产品边界：`BackendGate` 当前检查能力和策略；`execution.Adapter` 的 Bash 路径仍调用其装配的 `executor.Run`，不能把 gate 测试称作整套 UI 已改为执行 Loopback。本次直接后端测试和 Controller 重放测试证明本切片，不宣称所有应用输出路径有界。
- 回归：[output_tail_test.go](internal/sandbox/output_tail_test.go) 验证边界、绕回、并发、真实大输出、失败退出、取消以及事件重放。写入零分配测试和固定数组实现证明收集器不随输出量扩容；不是整个应用内存压测或吞吐提升数据。

stdout/stderr 仍是合并流，不保证并发进程之间的全局输出顺序；容量是两者合计 64 KiB，不是各 64 KiB。子进程仍可持续输出，受原有 timeout/cancel 控制；这里限制的是收集器保留的字节量，不限制子进程自己的内存，也不新增总输出配额或生产安全隔离。

## Q10：创建失败不等于资源已经清理

Q10 整项状态：2026-10-29 已对照原定验收关闭为 done，验收矩阵及 CI 证据见 [TASKS.md](TASKS.md)。本节说明最后一个修复切片；done 不扩大下文所述能力边界。

本切片由 `codex/q10-create-cleanup` 的 `44f37c6` 交付，已于 2026-10-29 经再次复核按维护者明确授权合入 main。背景是旧 `Controller.Create` 在后端返回错误后直接写入 `SandboxDestroyed`，并未调用后端确认清理；`Reconcile` 看到该事实就跳过，部分创建留下的目录可能因此漏清理。

现在把“创建失败”和“清理完成”分开：

1. `Controller.Create` 仍先记录原有 `SandboxInstanceCreated` 事实，再调用后端；这个历史名称不是后端已经就绪的证明。后端失败或返回不匹配的实例 ID 时，`cleanupFailedCreate` 记录 `SandboxCreateFailed`，随后对**请求推导出的实例 ID** 调用 `Destroy`，不对不可信的返回 ID 操作。
2. `Destroy` 成功才记录 `SandboxDestroyed`。清理失败时返回原始错误与清理错误，保留可重试状态；新建 Controller 读取同一 journal 后，仍能由 `Reconcile` 再次清理。这里的“重建”不等于新建一个 Loopback 后端后自动恢复目录所有权。
3. 两个状态投影都保留 `CreationFailed` 标记，阻止该代实例继续 `Exec`、重新 `Create` 或通过暂停/恢复绕过失败状态；清理入口仍可用。恢复清理成功后重复 `Reconcile` 不再操作后端。创建新实例应使用新的 generation，而不是复活已销毁的一代。
4. Loopback 用 `os.Mkdir` 原子创建实例目录，不接管已经存在的文件、目录或符号链接。记录目录归属后创建 `tmp`，全部成功才标记 ready；中途失败立即尝试清理，清理失败则保留归属供 `Destroy` 重试，不能执行任务。`removeOwnedDir` 检查登记与文件身份，拒绝删除无法确认归属的路径。

例如请求推导出实例 A，目录已创建但 `tmp` 初始化失败：如果目录删除成功，控制器确认清理后记录 Destroyed；如果删除失败，则保留 A 的失败标记和目录归属，下一次 Reconcile 重试，成功后再记录 Destroyed。若后端误返回实例 B，清理目标仍只能是 A，不能误删 B。

边界：journal 不可写时会返回记录错误，不吞掉错误或声称失败状态已持久化；这不是 journal 与文件系统的跨系统事务，也没有补齐崩溃时所有中间窗口。Loopback 的目录归属仍只存于当前后端内存，新后端遇到同名已有目录会拒绝接管/删除，需要另行恢复方案。文件身份检查也不构成对恶意并发替换路径的安全隔离。

回退注意：旧控制器不识别新增的 `SandboxCreateFailed`，会忽略这道执行限制。已经写入该事件且尚未完成清理的 journal，不能直接交给旧版本继续执行；应先使用新版完成清理，或隔离备份后使用新的测试 journal。本轮仅操作临时测试库，没有迁移真实用户数据或激活应用；新增事实可被旧代码忽略，不代表可以安全混用版本。

## Q11：可选 Pi 与启动解耦

2026-10-30：分支`3516dad`经独立复核，按维护者授权合入main。分支CI40996224960的Linux Sandbox普通/race各107条、启动普通/race各449条记录及macOS六项XCTest均通过、无跳过；合并提交CI另行核验，状态见STATUS。以下是当前实现与覆盖边界，不代表真实模型或完整桌面E2E已通过。

原来底层探测支持“没有找到 Pi”，但桌面入口先要求 Pi 安装目录与 Node 存在；参数解析、观察服务和执行服务装配又要求 Pi 搜索路径/实例 ID。结果是不使用 Pi 的用户也无法走完整启动链路。Q11 修复这条装配链，复用已有探测、原生适配器和权限机制，不重写 Q09/Q10。

```text
桌面启动 → localAppServiceArgsForHome
  ├─ Pi + Node 可用 → 保留原 Pi 参数、探测工厂与按需执行
  └─ Pi 缺失/不可执行或 Node 缺失 → 固定安全提示 + --disable-pi
       ↓
run 校验公共参数与互斥配置 → productionDaemonBuilder
       ↓
真实状态库/锁/IPC + 不含 Pi 的观察器 + 其他已配置后端
```

1. **公共资源不是可选的。**全新用户目录按层建立应用私有目录；已有目录不改权限。共享目录必须满足真实路径、类型、所有者和权限要求。目录或数据库异常仍失败，不能因为 Pi 可选而绕过安全校验。
2. **显式缺省，而非伪装配置。**`--disable-pi` 对应 `LocalRuntimeObservationDaemonConfig.DisablePi`，禁止同时传 Pi ID、搜索路径或本地模型目录。观察器保留真实 SQLite、锁、周期与退出处理，但不构造 Pi 工厂。空观察不会把其他后端改为离线，也不写伪造的 Pi 在线事实。
3. **其他后端仍走原授权路径。**`missionExecutionConfigFromDaemonBuild` 保留无 Pi 的产品执行装配；`buildProductMissionExecutionAPI` 不再要求无关 Pi 信息，仍拒绝半套或互相矛盾的配置。保留原有 EnrollmentOnly 工具接入，使用工具仍需账号策略和显式授权；没有安装依赖、自动选模型或扩大权限。
4. **历史事实不等于当前可用。**禁用 Pi 时，设置初始目录、刷新目录和已保存团队的重新绑定不再提供 Pi 候选；journal 不重写，原始历史视图仍可包含旧 Pi 记录。旧任务的执行仍需匹配可用适配器，不自动切到其他后端；历史预检不是现场执行证明。
5. **可选探测错误与系统错误分开。**沿用已有的 Pi 进程失败/超时/输出限额处理，补充格式错误、stderr 和重复模型等明确元数据错误；只有一个已知失败类别、唯一的版本/模型查询命令及允许的错误链才能保留 IPC。绑定变化、数据库、日志写入、未知清理错误或混合错误仍失败。后台上报固定原因码，不透出原始模型输出、凭据或用户路径。
6. **降级状态贯通到界面。**`recordFailure` 保存固定原因码，`applyRuntimeObservationHealth` 只接受对应 11 种码，Swift `closedReason` 与 `isUserVisibleDegradation` 保持同一集合。即使同时有“下一页”，探测异常也不能被误报为正常；只有正常分页仍显示在线。未知服务原因码不能绕过 API 校验。

例如全新机器没有 Pi/Node，但已配置另一个后端：桌面生成真实状态/连接路径和 `--disable-pi`，设置页能够读取，其他后端仍按原凭证与权限规则装配。若之后安装 Pi，需要重启服务重新发现；本次不是热插拔或自动安装。若共享状态目录被替换为符号链接，启动仍拒绝，不能降级为忽略该目录。

### 如何验证与哪些未验证

```sh
bash scripts/verify-startup.sh
bash scripts/verify-startup.sh --race
# 需要包含 XCTest 的 macOS 开发环境；Linux 不运行此入口。
bash scripts/verify-startup-ui.sh
```

[verify-startup.sh](scripts/verify-startup.sh) 先检查格式、锁定依赖、目标包编译和 vet，再执行固定的无账号回归集合并用原 `testreport` 校验器拒绝 skip/空包/未完成测试。覆盖首次启动组合、互斥参数、共享目录保护、实际服务构造到本地 IPC、原生适配器装配、历史 Pi 候选过滤、空观察重启及错误分类；测试使用临时库、测试工具脚本和假凭证，不调用真实模型。

[verify-startup-ui.sh](scripts/verify-startup-ui.sh) 固定执行28项XCTest：原六项Pi原因码/分页/超时降级、四项Roundtable错误兼容，加上完整18项诊断导出测试（原12项、3项凭证阶段及本次3项恢复阶段回归）。逐项要求实际通过记录，拒绝跳过和空选；它使用假客户端、临时日志与虚构错误，不启动真实后端或图形界面。`MissionWorkbench` 的长表达式只做等价分段以解除编译器类型推断瓶颈。

范围：Pi/Node 的启动耦合，不承诺任意第三方 CLI 错误均互不影响。显式错误配置仍拒绝；其他工具自己的依赖仍需满足。缺少 Pi/Node 的操作建议写入现有 daemon 控制台日志，不声称已新增 UI 安装向导。SwiftPM 应用编译已在本机通过，XCTest 的实际运行环境与结果见 STATUS；桌面操作、安装/签名/迁移、真实账号/E2E、工具修复后的自动热恢复未验证。原有 Sandbox 107 条回归不能替代本节的启动验证。

## CI 与 macOS 的边界

[sandbox-baseline.yml](.github/workflows/sandbox-baseline.yml) 在 main、feat/fix/docs/refactor/test/chore/perf 类型分支 push 和面向 main 的 PR 上运行同一入口的普通模式与 race 模式，兼容历史 codex 分支，两种模式都必须通过。race 检测会改变执行时序，不能替代普通模式。使用只读 contents 权限、固定 SHA 的 checkout、不持久化 Git 凭据、不注入真实模型密钥、不上传产物。维护者于 2026-10-27 授权仅在 GitHub CI 准备对应 Go 工具链：固定 SHA 的 `actions/setup-go` 从 `go.mod` 读取版本，必要时下载；不追踪 latest、不启用依赖缓存，然后下载锁定的公开 Go 模块。保留 `GOTOOLCHAIN=local`，后续命令不会隐式切换工具链；本机脚本仍离线、不安装 Go。下载失败或版本不满足时明确失败，不降低项目版本要求。CI 运行测试代码本身不等于网络隔离。

Q11 在同一工作流继续运行 `verify-startup.sh` 的普通/race 两种模式，原 Sandbox 两步保留；独立 `macos-15` 作业使用 runner 已有 Swift/Xcode 执行 `verify-startup-ui.sh`，不安装工具、不添加外部 Swift 包。两个作业都需成功。工作流文件存在不等于 Actions 已运行。未启用、需审批或缺少 runner 条件时，先记录事实并请求处理，不自动更改仓库权限或 Actions 设置。

macOS 的 `swift package --package-path apps/macos dump-package` 仍是独立的清单检查，不放进 Linux 作业，也不称作应用构建。继承的 build/install/live 脚本涉及应用构建、签名、服务启动或真实运行环境，未经核对不串入本入口。

## Q12：调试开关也不能输出原始错误

2026-10-30：`052ad2e`经完整及独立复核，按维护者明确授权合入main。分支CI41001765215已实际通过Linux普通/race与macOS六项测试，无跳过；合并CI另行核验，状态见STATUS。仅验收以下启动日志路径，Q12整项仍未完成。

既有实现已提供固定启动/运行原因码、凭证与配置修复建议、结构化运维诊断及导出字段过滤。本切片不重做这些能力，也不改变Q11的11类Pi健康原因码或Q10取消/超时语义。

真实缺口在[run.go](cmd/loomd/run.go)的builder失败路径：默认只返回`daemon unavailable: <固定原因码>`，但`LOOM_DEBUG_DAEMON=1`额外用`%#v`打印原始错误对象，错误文本或字段可能含用户路径、环境配置及授权值。这些日志还可能被桌面进程宿主收集，因此不能以“仅调试模式”为脱敏例外。

修复方式是移除原始对象日志分支，不用正则猜测哪些字符串像密钥。错误仍经原`daemonBuildFailureReason`映射为固定原因；非法或冲突分类保持`build_unknown`，退出码不变。调试开关不再为此构建失败路径提供原文详情，不新增字段、不改状态事实、凭证访问、授权或模型装配。

例如已由构建层归类为`build_state`的错误包含虚构的`Authorization`值和私人路径：无论调试开关是否开启，外部仍只能看到`daemon unavailable: build_state`；这表示定位到状态初始化阶段，不代表已经确定具体文件或认证原因。未知错误不能凭文本猜成认证失败，修复建议仍由已有明确分类链路给出。

[run_test.go](cmd/loomd/run_test.go)的`TestRunDebugBuildFailureDoesNotDiscloseDetails`与`TestRunDebugBuildFailurePreservesAllClosedReasons`覆盖普通/字段/包装/Join错误、非法与冲突原因、nil daemon、三种开关值和14类固定构建原因；严格断言stdout为空、退出码和stderr单行输出不变。另验证不调用未知错误的Error/GoString/Format，不在失败输出中打印原始内容。先在旧实现复现失败，再修复。

新增测试属于既有`TestRun.*`选择集合，直接通过`verify-startup.sh`普通/race进入现有CI，不增加平行脚本或跳过测试。测试只注入虚构错误，不读取真实凭据、不调用模型。范围仅为daemon构建失败日志，不宣称已审计整个应用的所有日志/界面路径；Q12整体验收仍需检查剩余真实缺口，分支交付不记done。

### 后续界面路径：Roundtable未知错误（2026-11-02）

圆桌切片06bca11已按维护者明确授权再次复核合入main；分支CI41380833279的普通/race与macOS十项已实际通过、无跳过，合并CI另验。不是整个Q12完成；下一已复现缺口为诊断导出的Vault阶段覆盖，见STATUS/TASKS。

`runRoundtable`捕获异常→`roundtableErrorMessage`生成字符串→`roundtableError`展示。原未知错误分支直接读取`localizedDescription`，可能显示用户路径、环境配置或授权值；固定提示替换这一出口后，不再求值未知错误的描述，也不把未知错误猜成认证/网络失败。提示检查本地连接再重试只是建议，不自动重试、有副作用的操作仍受原流程与权限限制。

类型化`LocalIPCRemoteError`继续沿原code/safeMessage/stage展示，包括conversationDispatch不追加stage的既有规则；本切片不声称能过滤任意错误中所有字段，不改远端协议或授权。成功操作清除旧错误的原路径未变。

本地使用`swift build`编译真实Core模块，再链接该模块对象调用实际函数的独立探针，虚构敏感NSError在旧实现失败、修复后通过，并验证类型化结果兼容。该探针不是XCTest，不能代替macOS CI；CLT缺少XCTest时明确记录限制。四项新增XCTest及原六项都在同一验证脚本逐项核验，Go普通/race仍由原CI执行。检查范围是这个界面出口，不等于全应用脱敏或真实GUI/E2E验收。

### 凭证诊断为什么写入成功却导不出来

2026-11-02：Vault切片`9df31e7`经重新完整及独立review无阻断，按维护者明确授权保留历史合入main。分支CI41383116299第二次运行的普通/race及25项XCTest均实际通过、无跳过；合并CI单独核验。以下实现/测试契约保留，末尾推送后核验属于交付时记录，不代表仍待审。

链路：Go凭证broker/Swift类型化错误 → 原诊断生产者 → 私有JSONL日志 → `LocalDiagnosticBundleExporter.readEvents` → 字段/结构/`safeEvent`与`safeStage`校验 → 预览 → `export`写出0600的JSON。导出器不读取Vault中的密钥、不给任何人新增凭证权限，只筛选已有诊断记录；存储格式与生产者均不变。

例如解锁失败写下incidentID、`vault_open`、`credential_unavailable`：旧白名单不含这个阶段，故在预览前整条被丢弃，支持人员便无法用incidentID关联故障。仅加入Go的`productOperationalDiagnosticStage`与Swift的`LocalIPCRemoteError.Stage`共同支持的15个凭证阶段（9个Vault、3个租约、3个迁移），原事件就能进入预览和JSON；不把所有枚举或任意前缀自动放行。

实际覆盖见[LocalDiagnosticBundleTests.swift](apps/macos/Tests/LoomLocalAppTests/LocalDiagnosticBundleTests.swift)：App通过真实`recordCredential`写30条，Daemon形状日志30条，核对来源/ID/阶段/耗时/结果/错误/可重试字段及JSON往返。额外保留provider与gateway兼容性；21种非法记录仍排除，包含未知阶段、非法结果组合、混合网关字段与敏感/嵌套原文。导出权限仍为0600，文件安全检查、网关规则、字段白名单、容量限制不变。

本地真实Core模块探针先失败（合法60条全部丢弃），修复后通过（合法60条保留、15条负向记录拒绝、JSON/权限一致）；这与新增XCTest的21种负向矩阵不是同一个计数。macOS CI通过既有脚本实际执行全部25项XCTest，不能把本地编译/探针或测试源码语法检查当作XCTest成功。原Linux普通/race检查保留；分支CI结果在推送后核验，Q12仍未整项验收，不自动合并main。

### 下一处已复现遗漏：恢复诊断没有进入导出

2026-11-02：`2a8b0b3`经完整及独立复核无阻断，按维护者明确授权保留历史合入main；分支CI41406327459普通/race与28项XCTest已实际成功无skip，合并CI单独核验。下方待审或下一次补恢复均为历史说明，不重复实现。

这是原有Agent/工具恢复功能的诊断链，不是重新开发Q10的取消或暂停恢复。`LocalIPCClient`调用恢复接口后，经原`recordAgentRecovery`/`recordToolRecovery`写私有日志；没有远端故障阶段时分别使用`agent_attempt_reconcile`/`tool_recovery`，有类型化故障则记录故障阶段。修复前`safeStage`未覆盖这两个默认阶段，因此日志能写入，预览却漏掉事件。

实际编译Core探针写入1条provider对照和4条恢复成功/失败记录，预览只保留provider并计4条skipped，导出JSON保持同一遗漏。下一次应为真实App写入及Daemon形状日志补预览/JSON回归，再最小补两阶段；安全拒绝规则不能退化。这里的恢复记录包含`recovery_operation`，但当前导出事件模型并不承诺保留该字段，本轮不把“补阶段”扩大为新增模型/权限。诊断遗漏是确认事实，不等于恢复执行功能本身失败；Q12尚未整项完成。

2026-11-02后续修复（待审分支`fix/recovery-diagnostic-export`）：先补三项XCTest，再只在`safeStage`加入上述两个固定值。原恢复请求、权限、日志生产者、字段/网关校验和JSON模型均未改；`recovery_operation`依旧不在导出字段内，不承诺新增操作明细。成功/失败诊断现在能携带原有incidentID/stage/result/error/retryability进入预览与JSON，非网关segment、非法组合、未知阶段和原文仍拒绝。

新增实际Core探针覆盖App/Daemon八条恢复记录和provider对照：旧代码仅1/9合法事件保留，修复后9/9保留、11条负向记录仍拒绝，JSON及0600一致；XCTest另有21种负向组合，与探针计数区分。Swift普通模块编译/运行及独立scratch构建的ThreadSanitizer模块/探针通过，TSan只验证所执行路径，不是全部XCTest或全应用竞态证明。本机CLT无XCTest，CI继续以28项实际XCTest与原Go普通/race作为交付条件；未安装依赖、放宽断言或跳过测试。

### Provider错误码为何仍需信任边界

当前修复（2026-11-02，分支`fix/provider-error-credential-redaction`的dcc3aa6经再次完整及独立review，按明确授权保留历史合入main）：下文“只核验、未实现”是修复前历史。`RespondConfigured`把本次借用的secret传给`classifyConversationHTTPFailure`→`conversationProviderErrorCode`→`safeConversationProviderCode`；最后一层先解码字符串或数值候选，在TrimSpace前排除包含完整本次secret的候选，再执行原长度/字符校验。OpenAI兼容和Anthropic两处实际调用均传入凭证，不在共享client保存它，也不延长凭证租约生命周期。

例如429响应中`error.code`回显本次虚构密钥，而`error.type`为`rate_limit_exceeded`：前者丢弃，后者按原候选顺序安全保留；若所有候选都回显或非法，ProviderCode留空，仍返回原`provider_rate_limit`、HTTP429、可重试和Retry-After18秒。`invalid_api_key`、数值1008及格式合法的未知码仍可诊断，不引入所有厂商错误码白名单或只按密钥前缀判断。

测试先在未改生产实现时实际RED，最小修复后GREEN。四种Provider经内存HTTP夹具检查四处候选、嵌入/JSON转义/数字凭证、正常码、安全回退和共享client请求隔离；集成用实际Provider→租约→Router→持久Chat API→回复/聊天JSON/operational.jsonl，检查无密钥及原始响应message，同时保留正常码与重试字段，租约副本清零。不是只构造一个过滤函数或手工伪造attempt。

`verify-startup.sh`保留原五包和全部原选择，新增Provider第六包、以上回归及原安全HTTP/诊断对照；本次普通/race各执行625条，校验器要求所有选定测试通过、无skip、无空包，而非将625硬编码为门槛。`verify-sandbox.sh`仍各107条；本地两入口普通/race与[分支CI41408923302](https://github.com/ggggggtm/ZoriDesk/actions/runs/41408923302)已实际通过，远端macOS28项也通过无skip，锁文件未改；本次合并CI单独核验。本轮review不机械重跑同一未变化的本地基线，不把引用证据当作新运行。本切片不改Swift，不宣称任意编码/截断/其他凭证检测、真实GUI或模型E2E通过。

Q12整项完成依据见[TASKS验收矩阵](TASKS.md#q12让诊断可理解可操作)：只关闭原定分类/建议及已确认隐私与导出缺口，不是全应用安全审计。后续出现具体新失败才修对应回归；未承诺的Agent Input完整导出不阻塞本轮有界收口，也不因关闭任务放宽现有负向规则。

修复前，固定的安全消息不代表所有附带元数据都安全。实际链路是外部响应error.code → `ConversationFailureDetails.ProviderCode` → daemon的conversation适配 → API安全字段/失败记录 → Swift错误详情拼接或运维诊断。此前外部码只过长度/字符校验；若异常上游回填请求凭据，而该凭据恰好符合字符规则，它会被当作可展示诊断码。

本轮在隔离Git副本中使用真实`NewDeepSeekConversationClient.Respond`，注入假HTTPDoer：读取虚构Authorization，只在内存返回401及含同一虚构值的error.code。实际Provider解析结果及`NewLocalProductConversationDispatchErrorWithDetails`均保留该值，固定Error()/UserMessage不含该值；没有真实网络/密钥/Vault，也未执行GUI或日志出口。所确认的是条件性元数据边界缺口，不是线上泄露。

下一回归应覆盖这条回显数据流与诊断出口，同时保留合法字符串/数值码、HTTP分类、retry和固定安全提示；至少阻止已知请求凭据进入诊断码，再明确未知码回退策略，不能只按某个前缀或空格判断。Agent Input的非网关segment则有现存拒绝契约，需先决定支持范围，不混为这条隐私修复的一部分。本轮只核验并记录，未实施新Provider行为。

## Q06：展示名称与兼容标识分开

2026-11-03验收更新：b722a1c经再次完整及新的独立只读review无阻断，按维护者明确授权保留历史合main；[分支CI41630842954](https://github.com/ggggggtm/ZoriDesk/actions/runs/41630842954)准确SHA及原始日志重新核对，普通/race启动625条、Sandbox107条和31项XCTest均无skip，三项品牌测试逐项passed。本次合并CI单独确认；以下推送前描述为历史证据，不能误判仍需重写头部或替代合并CI。

首切片`feat/workspace-branding`只统一WindowGroup标题、工作区/Mission侧栏头部与无障碍名称。`LocalProductCopy.productName`是展示文案，不是协议、bundle ID或存储namespace；入口与两处头部引用它，纯SwiftUI的`WorkspaceBrandHeader`保留原52点高度、图标/字体/内边距，宽版显示名称、紧凑版仅显示图标但保留无障碍名称。这里不需要创建Store、启动daemon或访问Vault。

先增加三项XCTest，再对旧生产源码执行接线检查，因窗口Loom而非ZoriDesk真实失败；修改后同一检查通过。该RED是源码契约，不是GUI测试执行。实际Swift应用编译通过，独立探针链接真实Core/UI模块，在192/64点宽、明/暗四种组合离屏渲染，PNG及布局检查通过并逐张查看名称/图标无截断；不是复制一个假界面，也不是安装后整窗或菜单验收。

`verify-startup-ui.sh`保留原28项并新增`LoomGraphiteViewTests`三项：文案、生产接线/兼容标识、四种NSHostingView布局与位图，共31项逐项核验passed、拒绝skip/空选。本机CLT实际无法导入XCTest，不能把编译/探针算成XCTest；上述对应提交CI实际执行全部检查。本轮review只引用未变化的本地证据，不伪称重新运行。Q06仍有其他可见文案待分批核对，不全局替换Loom、不触及Q07/Q08安装与数据兼容。

剩余核验见[TASKS的Q06清单](TASKS.md#q06统一用户可见品牌)：活跃聊天展示/读屏/复制文本优先，其后连接与错误建议、Vault展示和包显示元数据；本次只有源码审计，没有运行安装后菜单或改这些行为。下一项改品牌展示时必须保留`"loom"`角色协议值、用户内容、proposal不可信提示和原重试/权限规则；`.loomvault`格式、运行时别名含义、存储与来源不是任意替换对象。

### 聊天展示不是角色协议重命名（2026-11-04）

验收更新：2be683d完整八文件经再次及新的独立只读review无阻断，按维护者授权保留历史合main；[分支CI41765715150](https://github.com/ggggggtm/ZoriDesk/actions/runs/41765715150)准确SHA和原始日志已重核，普通/race与38项XCTest无skip，合并CI另验。源码/测试/脚本不再改，本轮引用既有本地探针，不称重新执行。下方“待审”是交付时状态，不能据此重复前缀实现。

分支`fix/chat-presentation-branding`复用`LocalProductCopy.productName`定义聊天名称、占位和回复中标签；真实timeline、TextField、ProgressView读屏及复制转录通过展示常量/`conversationSpeakerLabel`/`conversationMessageAccessibilityLabel`取值。原`"user"`仍展示You，`"proposal"`显示ZoriDesk proposal且读屏保留untrusted；`"loom"`、confirmation及原未知角色fallback展示助手名称，但不改任何原始role值。proposal身份不依赖tentative；用户读屏原文与其他角色displayContent、工具形态警告规则保持。

新增四项回归，同步旧转录预期与旧builder安全接线断言：仍验证proposal不可信、角色判断及其余原断言，不删除测试。原Stop按钮行为纳入门禁，原31项完整保留后总计38项，逐项要求passed、拒绝skip和空选。JSON前后编码/解码、中文/换行/空白/原文Loom、空转录、角色fallback、proposal tentative true/false及工具形态消息均有测试。发送/复制/Profile切换函数逐字对照未变，不改正文、段落分隔或剪贴板动作。

本地实际编译旧Core/UI调用`conversationTranscriptText`先RED（旧前缀不符）；最小修复后同探针GREEN，实际展示/读屏标签函数与Stop动作探针通过，应用编译通过。探针不实例化Store、不读取用户数据/凭证，不是完整聊天GUI、实际VoiceOver或E2E；本机CLT确实缺XCTest，正式38项及原Go普通/race由推送后的对应CI实际执行。Q06整体仍进行中，当前分支待审、不合main；窗口/侧栏不再修改，连接/错误、Vault、包元数据未在本切片改动。

本次只读剩余审计还确认了路由披露/权限与恢复说明、Roundtable创建引导、工具形态安全警告正文的旧名，均有生产展示调用。后者是前缀切片刻意保留的内容，不是未完成同一个前缀修复；后续只能改品牌词并保留安全语义。具体清单见[TASKS的Q06范围](TASKS.md#q06统一用户可见品牌)，不是安装后菜单或真实VoiceOver实测。下一次先选连接状态小切片，不动已验收聊天、Q12诊断逻辑或Q07/Q08标识，本轮不顺带实现。

### 六处连接展示，不改连接状态机（2026-11-04）

验收更新：d91923e完整八文件经再次及新的独立只读review无阻断，按维护者明确授权保留历史合main。[分支CI41769618780](https://github.com/ggggggtm/ZoriDesk/actions/runs/41769618780)准确对应d91923e，已读取原始日志：普通/race启动各625条、Sandbox各107条，macOS编译及43项XCTest通过无skip，三项新测试逐项passed。本次合并CI单独核验，不借用分支成功；本轮不重跑未变化的本地基线。下方“待审”仅为原交付状态，不能据此重复实现。

分支`fix/connection-status-branding`仅在LocalProductCopy集中四条连接detail及Mission连接标题/loading横幅，原四个Core分支和两处Mission位置引用常量。loading/online/partial/offline的品牌词更新；stale、offline保留快照、fatal等非品牌文案及所有state/title/icon/offersRefresh、内容映射保持。Mission的snapshot判断和authorityBanner各分支/刷新入口未重构，也不改ContentView启动、刷新与恢复并行顺序。

三项新回归与两项既有检查追加到原38项门禁后共43项：四条品牌词和两个Mission常量、nil/空/非空快照的状态与内容保留、真实Mission接线，以及原私有reason拒显和ContentView恢复顺序。仍逐项要求passed并拒skip/空选。另逐字源码对照验证，排除精确4+2处展示引用替换后，Core映射和整个Mission文件等同main。

本地旧实际Core探针RED退出1，修复后同探针在12个nil/保留快照状态场景GREEN，另Mission标签常量检查通过；修改后应用构建通过。没有实例化Store、调用真实刷新/恢复或连接服务，不能称网络恢复或安装后GUI E2E通过；CLT缺XCTest，正式43项及原Go普通/race交由对应提交CI实际执行。当前待审、不合main，Q06仍有其他文案未完成，不重复窗口/侧栏/聊天或更改Q12逻辑。

本次剩余核验仅做源码与生产调用链检查：Mission设置恢复提示、执行失败与chatFailure等故障建议；路由披露/权限/Roundtable说明；toolShapedWarning中的品牌词；Vault页面/导出建议文件名前缀；Info.plist两个显示名称仍未统一，见[TASKS的Q06范围](TASKS.md#q06统一用户可见品牌)。这不是安装后界面实测；工具警告必须保留“未执行”，Vault扩展名/格式及服务路径等兼容标识不在机械替换范围。没有顺带实现下一切片，Q06不能标done。

### 故障与恢复提示只改显示名（2026-11-05）

验收更新：c30d7be完整10文件经再次及独立只读review无阻断，按明确授权保留历史合main。[分支CI42003275772](https://github.com/ggggggtm/ZoriDesk/actions/runs/42003275772)准确对应c30d7be；读取原始日志确认Go普通/race各107条Sandbox、625条启动/Provider及macOS编译/55项XCTest通过、无跳过，追加的12项逐项通过。合并CI另验；不重跑未变化的本地基线。

`fix/failure-recovery-branding`基于main 513af61。Mission的`missionRuntimeRecoveryReason`、`missionSetupRecoveryPresentation`和执行失败标签按原状态输出；`runtimeAvailabilityDetail`按原可用性输出；`LocalProductStore`在原失败映射中只替换固定提示内的产品词。对照基线，3份源码仅61行中73处品牌词变为`LocalProductCopy.productName`，其余字节不变。聊天`chatFailureDetail`仍优先显示后端的非空安全消息，原样附HTTP状态、Provider码和重试时间；Agent/工具必须按既有确认和候选校验流程处理。后端生成的文案不在本次范围，收到的消息不做全局替换。

实际旧Core/UI探针10项品牌断言失败、7项行为对照通过；新应用编译成功后，同探针17项全过。探针通过假客户端与临时会话文件验证Mission/Runtime/侧任务、聊天迁移失败与安全消息/重试字段、Agent恢复超时，未访问真实凭证或服务。新增4项XCTest覆盖设置/预检、账户/费率/工具失败、聊天安全消息及Agent恢复确认；另纳入8项既有文案与保护回归。`verify-startup-ui.sh`保留原43项、逐项核对全部55项，不允许跳过或空选。本机CLT无XCTest，探针与源码语法检查不是正式XCTest。

`verify-sandbox.sh`和`verify-startup.sh`本地普通/race分别各107、625条记录均通过无跳过，Go源码、工作流与锁文件不变。此次不是完整GUI、实际恢复执行或模型E2E验收。桌面提示切片已复核、不重复；Q06仍进行中，剩余见[TASKS](TASKS.md#q06统一用户可见品牌)。

剩余提示的来源已区分：Go `conversationFailureUserMessage` → 聊天API的安全消息/回复 → Swift `chatFailureDetail`，不能靠桌面fallback统一后端文案；远程工具绑定的预检`block_reason`由Mission直接显示，同时整个预检对象参与摘要。工具形态警告由Go API替换回复、Swift过滤器兜底，后续需区分新消息和历史文本，不改变拒绝执行语义。以上仅源码核验，未改余项的生产代码或做安装后界面实测。

### HTTP 成功不等于 Provider 回复有效（2026-11-05）

验收更新：1b49591完整8文件经再次完整及独立只读复核无阻断，按授权保留历史合main。[分支CI42022223574](https://github.com/ggggggtm/ZoriDesk/actions/runs/42022223574)准确对应1b49591，原始日志确认Go普通/race启动各626条、Sandbox各107条，以及macOS编译与55项XCTest通过无跳过；新增跨组件用例两种Go模式均执行。合并CI另验，本轮未重跑同一源码的本地基线。

`fix/provider-response-branding`只在`conversationFailureUserMessage`替换`invalid_response`的固定品牌词。比如HTTP 200但助手内容为`null`，`invalidConversationHTTPResponse`仍给出`provider_http / invalid_response / response_content`且不可重试。Anthropic的模型/内容块校验也使用同一固定消息生成器。Router将固定消息传给聊天API，API生成可见回复并保存Attempt失败详情；Swift仍按原规则优先使用安全消息。没有修改原始响应解析、凭证、错误元数据、权限、重试、历史记录或预检摘要。

既有OpenAI兼容及Anthropic用例加强品牌和分类断言；`TestConversationProfileRouterShowsBrandedInvalidResponse`经真实客户端、内存HTTP、Router与临时聊天API核验回复和Attempt（含HTTP200、事件号与非重试），同时检查API JSON和临时聊天存储无虚构原文/凭证。先在未改生产时观察四个场景因旧品牌失败，最小修复后同回归通过。`verify-startup.sh`只追加这一个测试选择，本地普通/race各626条，Sandbox各107条，无跳过；格式、锁文件和工作流范围检查通过。未调用真实Provider或验证安装后GUI；已验收固定句不再重复开发。

余项核验：新绑定校验已拒绝不支持的适配器，旧提示属于兼容性防御路径，未现场复现。Mission主/回退预检节点分别直接显示`blockReason`、`fallbackBlockReason`，Swift节点没有独立`blockCode`，原Go预检参与摘要。若修复，在展示层精确映射已知固定旧句，保留未知文本、原字段和审批；不修改后端理由或重算摘要。诊断导出的`loom-diagnostics.json`是保存面板建议名，与Vault建议名均未改动。其余分类见[TASKS](TASKS.md#q06统一用户可见品牌)，没有顺带实现或实测安装后菜单。

### 对话路由披露说明只改产品名（2026-11-05）

验收更新：4bb61a4完整差异经再次及独立只读复核无阻断，按授权保留历史合main。[分支CI42030392545](https://github.com/ggggggtm/ZoriDesk/actions/runs/42030392545)准确对应4bb61a4；原始日志中Go普通/race各107条Sandbox、626条启动/Provider，macOS编译及58项XCTest均通过，无跳过，三项路由测试逐项通过。合并提交CI独立核验，生产源码、测试和脚本与已审分支一致。

`ConversationRouteTransitionSheet`接收待确认的路由变更，默认选「Summary only」。只有存在已冻结的源执行绑定，并且没有信任边界变更或用户已确认这些变更，`conversationRouteTransitionConfirmationEnabled`才允许确认。确认仍把所选上下文模式和确认状态交给`LocalProductStore.confirmConversationRouteTransition`，成功才关闭弹窗。`fix/route-disclosure-branding`只在「Start clean」详情和盾牌图标旁的回执说明引用`LocalProductCopy.productName`，不改变共享范围、凭证隔离、确认或回执逻辑；整个生产文件对照main b70bb2b除这两处外逐字相同。

交付时用Foundation运行同一源码回归的两条断言，旧文案0/2、新文案2/2；本地应用构建、Go普通/race及语法检查通过。`verify-startup-ui.sh`保留原55项，追加文案、离屏渲染与确认门槛三项，正式XCTest由macOS CI执行。未做安装后GUI、实际路由派发或真实模型E2E。本切片不再待审，Q06整体仍进行中；本轮仅核实[TASKS余项](TASKS.md#q06统一用户可见品牌)，未改其他生产路径。

### Roundtable 创建引导只改产品名（2026-11-06）

验收更新：1bcb257经再次及独立只读复核，按授权保留历史合main。[分支CI42174092042](https://github.com/ggggggtm/ZoriDesk/actions/runs/42174092042)准确对应1bcb257，原始日志确认Go普通/race各107条Sandbox、626条启动/Provider，macOS编译及60项XCTest全部执行通过、无跳过。合并CI独立核验，生产、测试和脚本沿用已审分支。

未加载会话时显示`RoundtableWorkbench.createSessionForm`。创建按钮仍通过`createSession`调用Store并刷新快照，席位与后续 propose → relay → acknowledge → insert → conclude 流转保持原样。`fix/roundtable-session-copy`对照main 48a26f0，生产源码只替换引导`Text`中的一个产品词；不改按钮、文案的步骤说明、权限或API。

交付时用Foundation复跑实际源码断言：旧版8条席位断言通过、1条品牌断言失败，修复后9/9。Swift应用构建及语法检查通过；Go `internal/roundtable`本地普通/race各13条、无跳过。保留原58项macOS门禁，新增文案与既有席位检查共60项，不是安装后GUI或真实消息派发。该引导不再待审，Q06其余[TASKS范围](TASKS.md#q06统一用户可见品牌)未顺带修改。

### 导出建议名不是实际存储位置（2026-11-07）

验收更新：396cba2完整差异经再次及独立只读复核无阻断，按授权保留历史合main。[分支CI42349742248](https://github.com/ggggggtm/ZoriDesk/actions/runs/42349742248)准确对应396cba2；原始日志核实Go普通/race各107条Sandbox、626条启动/Provider和macOS编译、63项XCTest通过，无跳过。合并提交CI独立核验，生产、测试、脚本沿用已审分支，不重复修改。

`CredentialVaultExportSheet.chooseDestinationAndExport`先检查`canExport`，再弹出限定`.loomvault`类型的保存面板；确认后使用用户选定的`panel.url?.path`和原口令调用Store。`DiagnosticBundlePreviewSheet.exportBundle`仍限定JSON，确认后把所选URL和已审预览交给原Exporter。`fix/export-filename-branding`只新增两个`LocalProductCopy`建议名，并替换两处面板初始值，不自动导出、不替用户选路径、不更改格式、权限或内容。旧文件名和自定义目标仍可使用，既有测试里的旧目标路径刻意保留。

交付时旧源码接线仅两个品牌断言失败，新版14/14；实际展示枚举/测试体普通及ThreadSanitizer探针各8/8，核对新名字、扩展名及无路径分隔符。探针不等于全应用并发保证或正式XCTest。本地Swift构建、语法/脚本及Go Sandbox普通/race通过；CI保留原60项并追加三项选择，逐项要求63项passed且拒绝skip/空选。未访问真实Vault、弹出保存面板或验证完整GUI。建议名切片不再待审，Q06余项另按TASKS分批处理，未顺带实现。
