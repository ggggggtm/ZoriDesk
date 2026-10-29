# 无账号的 Sandbox 验证入口

状态：Q05 的首个有界切片，尚未代表完整应用通过验收。运行结果见 [STATUS.md](STATUS.md)，任务依赖见 [TASKS.md](TASKS.md)。

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

本切片由 `codex/q10-create-cleanup` 的 `44f37c6` 交付，已于 2026-10-29 经再次复核按维护者明确授权合入 main。背景是旧 `Controller.Create` 在后端返回错误后直接写入 `SandboxDestroyed`，并未调用后端确认清理；`Reconcile` 看到该事实就跳过，部分创建留下的目录可能因此漏清理。

现在把“创建失败”和“清理完成”分开：

1. `Controller.Create` 仍先记录原有 `SandboxInstanceCreated` 事实，再调用后端；这个历史名称不是后端已经就绪的证明。后端失败或返回不匹配的实例 ID 时，`cleanupFailedCreate` 记录 `SandboxCreateFailed`，随后对**请求推导出的实例 ID** 调用 `Destroy`，不对不可信的返回 ID 操作。
2. `Destroy` 成功才记录 `SandboxDestroyed`。清理失败时返回原始错误与清理错误，保留可重试状态；新建 Controller 读取同一 journal 后，仍能由 `Reconcile` 再次清理。这里的“重建”不等于新建一个 Loopback 后端后自动恢复目录所有权。
3. 两个状态投影都保留 `CreationFailed` 标记，阻止该代实例继续 `Exec`、重新 `Create` 或通过暂停/恢复绕过失败状态；清理入口仍可用。恢复清理成功后重复 `Reconcile` 不再操作后端。创建新实例应使用新的 generation，而不是复活已销毁的一代。
4. Loopback 用 `os.Mkdir` 原子创建实例目录，不接管已经存在的文件、目录或符号链接。记录目录归属后创建 `tmp`，全部成功才标记 ready；中途失败立即尝试清理，清理失败则保留归属供 `Destroy` 重试，不能执行任务。`removeOwnedDir` 检查登记与文件身份，拒绝删除无法确认归属的路径。

例如请求推导出实例 A，目录已创建但 `tmp` 初始化失败：如果目录删除成功，控制器确认清理后记录 Destroyed；如果删除失败，则保留 A 的失败标记和目录归属，下一次 Reconcile 重试，成功后再记录 Destroyed。若后端误返回实例 B，清理目标仍只能是 A，不能误删 B。

边界：journal 不可写时会返回记录错误，不吞掉错误或声称失败状态已持久化；这不是 journal 与文件系统的跨系统事务，也没有补齐崩溃时所有中间窗口。Loopback 的目录归属仍只存于当前后端内存，新后端遇到同名已有目录会拒绝接管/删除，需要另行恢复方案。文件身份检查也不构成对恶意并发替换路径的安全隔离。

回退注意：旧控制器不识别新增的 `SandboxCreateFailed`，会忽略这道执行限制。已经写入该事件且尚未完成清理的 journal，不能直接交给旧版本继续执行；应先使用新版完成清理，或隔离备份后使用新的测试 journal。本轮仅操作临时测试库，没有迁移真实用户数据或激活应用；新增事实可被旧代码忽略，不代表可以安全混用版本。

## CI 与 macOS 的边界

[sandbox-baseline.yml](.github/workflows/sandbox-baseline.yml) 在 main、codex 分支 push 和面向 main 的 PR 上依次运行同一入口的普通模式与 race 模式，两者都必须通过。race 检测会改变执行时序，不能替代普通模式。使用只读 contents 权限、固定 SHA 的 checkout、不持久化 Git 凭据、不注入真实模型密钥、不上传产物。维护者于 2026-10-27 授权仅在 GitHub CI 准备对应 Go 工具链：固定 SHA 的 `actions/setup-go` 从 `go.mod` 读取版本，必要时下载；不追踪 latest、不启用依赖缓存，然后下载锁定的公开 Go 模块。保留 `GOTOOLCHAIN=local`，后续命令不会隐式切换工具链；本机脚本仍离线、不安装 Go。下载失败或版本不满足时明确失败，不降低项目版本要求。CI 运行测试代码本身不等于网络隔离。

工作流文件存在不等于 Actions 已运行。未启用、需审批或缺少 runner 条件时，先记录事实并请求处理，不自动更改仓库权限或 Actions 设置。

macOS 的 `swift package --package-path apps/macos dump-package` 仍是独立的清单检查，不放进 Linux 作业，也不称作应用构建。继承的 build/install/live 脚本涉及应用构建、签名、服务启动或真实运行环境，未经核对不串入本入口。
