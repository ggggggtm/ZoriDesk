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
- [loopback_test.go](internal/sandbox/loopback_test.go)：在临时目录执行本地 shell，覆盖命令、取消和目录清理。Loopback 是宿主机进程，不提供强隔离。这些已有断言不等于完整验证环境变量泄漏和全部进程树回收；Q09/Q10 仍需加强边界场景。
- [reconcile_test.go](internal/sandbox/reconcile_test.go)：临时 SQLite journal + fake backend，验证事实记录、失败重放、generation 拒绝、状态重建和清理幂等；不读取真实工作台数据库。

未覆盖：全仓编译/测试、Swift 应用构建与 UI、安装/迁移、真实模型 E2E、Docker/Kubernetes、完整输出上限及进程树取消语义。不要以本入口通过替代这些验收。

## CI 与 macOS 的边界

[sandbox-baseline.yml](.github/workflows/sandbox-baseline.yml) 在 main、codex 分支 push 和面向 main 的 PR 上运行同一入口的 race 模式。使用只读 contents 权限、固定 SHA 的 checkout、不持久化 Git 凭据、不注入真实密钥、不上传产物。只下载锁定的公开 Go 模块，使用 runner 现有工具链；版本不足或依赖不可用时明确失败，不自动安装或升级。CI 运行测试代码本身不等于网络隔离。

工作流文件存在不等于 Actions 已运行。未启用、需审批或缺少 runner 条件时，先记录事实并请求处理，不自动更改仓库权限或 Actions 设置。

macOS 的 `swift package --package-path apps/macos dump-package` 仍是独立的清单检查，不放进 Linux 作业，也不称作应用构建。继承的 build/install/live 脚本涉及应用构建、签名、服务启动或真实运行环境，未经核对不串入本入口。
