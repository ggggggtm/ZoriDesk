# 参与 ZoriDesk

欢迎提交可复现的问题、小范围修复和有明确使用场景的改进。ZoriDesk 基于 Loom 演进；保留 [Apache-2.0](LICENSE)、必要版权、来源与真实历史，独立贡献由实际差异和验证记录体现。

## 先确认范围

1. 阅读 [当前状态](STATUS.md)、[任务队列](TASKS.md) 和 [维护规则](AGENTS.md)，检查相关分支及待审工作，避免重复实现。
2. 阅读目标目录适用的 `AGENTS.md`，再检查相关代码、测试与契约。继承的 `docs/CURRENT.md`、`PROGRESS.md` 和 `.loom-evidence/` 不是本项目已验证结果。
3. 一次变化解决一个可以验收的问题。涉及凭证、授权、状态写入、安装或数据迁移时，先说明信任边界、兼容方案与回退方式，再进入实现。

普通对话不自动授权执行；模型输出不是执行权限。其余安全不变量以根 `AGENTS.md` 为准，不在这里维护第二份规则。

## 问题与建议发到哪里

唯一项目目标是 [ggggggtm/ZoriDesk](https://github.com/ggggggtm/ZoriDesk)。这是一个 fork，GitHub 创建 PR 时可能默认选择上游：提交前必须核对 **base repository 为 ggggggtm/ZoriDesk，base branch 为 main**，不要误发到 Loom 上游。

**2026-10-13 核验：本仓库 Issues 和 Discussions 尚未启用。** 缺陷与功能模板先随源码提供；模板合入默认分支、且维护者启用 Issues 后，才可以用于网页提报。当前不要寻找不存在的提报入口或向上游代发：

- 缺陷：先用 [缺陷模板](.github/ISSUE_TEMPLATE/bug_report.md) 整理可复现、已脱敏的记录。
- 建议：先用 [功能模板](.github/ISSUE_TEMPLATE/feature_request.md) 写清使用场景、替代方案与验收条件。
- 已有适当的维护者沟通渠道时，可发送上述记录；没有时先保留草稿，等待维护者提供渠道。不要为普通反馈创建空 PR。
- 有实际修复或文档改进时，可按下文准备 PR，不要求必须关联一个 Issue。自动维护是否创建 PR，仍服从用户单独授权；推送分支不等于获准创建 PR。

不要自动开启 Issues、创建标签、修改权限或配置外部服务。本文不承诺私人安全报告渠道已经存在；涉及凭证泄漏或可利用漏洞时，不在公开 Issue、PR 或日志中披露细节，先与维护者确认合适的私密渠道。

## 准备开发环境

已有专用 checkout 时复用，不覆盖脏工作树。尚未克隆时：

```sh
git clone https://github.com/ggggggtm/ZoriDesk.git
cd ZoriDesk
```

修改前检查真实路径、远端、当前分支和未提交改动：

```sh
git rev-parse --show-toplevel
git remote -v
git status --short --branch
git config --get user.name
git config --get user.email
```

确认提交邮箱是否适合公开。网页、连接器和 Git 的登录身份相互独立，不要把浏览器登录当作 Git 写权限；不要为本项目覆盖全局 Git 身份、关闭安全钩子或把令牌写入远端 URL。

Go 要求以 [go.mod](go.mod) 为准（当前声明 1.25.5）；macOS 包以 [Package.swift](apps/macos/Package.swift) 为准（tools 6.0、macOS 14）。当前环境的实测结果见 `STATUS.md`，不要据此承诺 Windows/Linux 桌面应用可用。Pi、模型账号或 Docker 不是以下最小检查的前置条件；真实运行工具与完整应用仍需各自验证。

## 提交一个小改动

有写权限的维护者使用 `codex/` 前缀独立分支；外部贡献者从自己的 fork 提交。先检查未完成工作，不机械新开分支。同一分支只保留一个写入者；不覆盖别人的修改、不强推、不改写历史。

1. 写明问题、修改范围和验收条件。运行行为改动先补一个能复现问题的回归测试；纯文档改动检查内容、链接与格式即可。
2. 实现最小变化，不顺带重命名模块、升级依赖或迁移真实数据。
3. 执行匹配风险的检查，保留完整命令、结果、环境与未覆盖范围。用临时测试目录或虚构样例，不操作真实凭证和用户数据。
4. 审查差异与敏感信息，再以真实完成时间提交；提交说明采用 `type(scope): 描述`，复杂变化在正文说明原因和验证。不制造空提交、拆碎活动或倒签日期。
5. PR 使用 [审查模板](.github/pull_request_template.md)。说明兼容影响、失败处理和回退方式；文档任务不适用的项写明原因，不用假勾选代替证据。

自动维护允许推送已授权的本项目改进分支，但不自动合并 main。队列任务在待审时标记 `review`，验收并合入 main 后才是 `done`。软件运行时 WorkItem 的验收另受其契约约束。

## 可以从哪些检查开始

Q05 已新增可执行的 [Sandbox 验证入口](VERIFICATION.md)：`bash scripts/verify-sandbox.sh`（可加 `--race`）。它覆盖有界 Go 包并拒绝跳过/空测试集；合并与 CI 验证状态以 STATUS.md 为准，不代表整个应用通过。

以下命令从仓库根目录执行，使用现有工具链与缓存，不主动下载新依赖。`GOPROXY=off` 并不代表应用中任意测试都没有网络行为，因此这里只列已核对的最小范围，不推荐盲跑真实模型 E2E。

```sh
git diff --check
GOPROXY=off GOTOOLCHAIN=local go mod verify
GOPROXY=off GOTOOLCHAIN=local go test -mod=readonly ./internal/sandbox -count=1 -timeout=90s -v
swift package --package-path apps/macos dump-package
```

| 检查 | 能说明什么 | 不能说明什么 |
| --- | --- | --- |
| `git diff --check` | 差异中的空白错误检查 | 内容准确、链接可用或代码正确 |
| `go mod verify` | 已下载模块缓存通过完整性校验 | 已下载全部依赖或测试通过 |
| sandbox 包测试 | 现有接口、Loopback 与状态记录的包级回归 | 所有异常已覆盖、全仓通过或生产隔离安全 |
| Swift 清单解析 | 工具链可解析包定义 | 应用编译、安装或界面通过 |

缺少依赖时写明“未执行到测试”及具体原因。这属于环境阻塞，不能据此判断测试通过或功能缺陷已修复。经授权才下载锁定依赖；不要用升级版本、自动装工具或移除测试绕过问题。Q05 已提供有界验证入口和 CI 配置；实际 CI 结果以 STATUS.md 记录为准，不把配置存在称作完整 CI 通过。

纯文档改动无需为了提交重复运行应用或模型；Go、Swift、安装或执行行为改动需要相应的额外检查。`skip`、缓存成功、mock 成功与真实集成成功必须分别描述。继承脚本执行前先阅读副作用，不自动安装服务、拉取容器或调用付费模型。

## 公开材料与脱敏

- 不提交公司内部代码、设计、工单、日志、私有地址或未经许可的数据。
- 不提交 API key、令牌、Cookie、SSH 私钥、凭证数据库、真实会话、提示词或环境变量转储。
- 日志仅摘录相关片段；将用户名、绝对路径、会话标识及业务数据替换为稳定的虚构值，保留错误类型、相对时序和复现条件。截图也需检查地址栏、侧栏和账户信息。
- 使用公开上游材料时保留必要的来源和许可；不要把继承实现或上游验证结果写成本项目新增成果。

模板格式依据 [GitHub 官方模板说明](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/about-issue-and-pull-request-templates)；目录与元数据静态检查不等于 GitHub 页面已经启用或完成交互验证。
