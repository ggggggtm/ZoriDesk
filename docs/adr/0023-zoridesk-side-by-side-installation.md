# ADR-0023：ZoriDesk 并存安装与数据隔离

**Date**: 2026-11-16
**Status**: proposed（TARGET，未实现）
**Deciders**: ZoriDesk 维护者，待评审

## Context

**CURRENT**：Q06 统一了显示名，不改变安装、后台服务或数据归属。现有 ZoriDesk 构建仍与 Loom 共享默认位置；不能把这个构建当作可与旧 Loom 并存的交付。以下盘点来自 `fc09c8f` 的源码，不是本机现有安装调查。

| 边界 | 当前默认值与调用方 | 候选 ZoriDesk 值（未注册、未启用） |
| --- | --- | --- |
| 应用安装 | `Loom.app`、`com.earendilworks.loom.local`；[构建](../../scripts/build-loom-local-app.sh)和[安装/回退](../../scripts/install-loom-local-app.sh)验证包名、标识和签名 | `~/Applications/ZoriDesk.app`、`io.github.ggggggtm.zoridesk.local`；不接管旧包或安装收据 |
| 随包用户服务 | [LaunchAgent](../../apps/macos/Resources/com.earendilworks.loom.local.daemon.plist) 的文件名/Label、[SMAppService](../../apps/macos/Sources/LoomLocalAppCore/LocalServiceBootstrap.swift) 与[前台回退](../../apps/macos/Sources/LoomLocalApp/LocalServiceProcessHost.swift) 都认 `com.earendilworks.loom.local.daemon`；helper 为包内 `loomd` | `io.github.ggggggtm.zoridesk.local.daemon`；注册与回退使用同一新身份，只运行匹配的包内 helper |
| 状态、执行、运行时 | [随包 daemon](../../cmd/loomd/local_app_service.go) 在 `~/Library/Application Support/Loom` 下派生 `state/loom.db`、`isolation/`、`run/loomd.sock` 和可选 `runtimes/pi/`；[IPC 锁](../../internal/localipc/socket.go) 随 socket 派生 | 同一组相对位置放在 `~/Library/Application Support/ZoriDesk` 下；不沿用旧 Pi 缓存、锁、活动进程或 socket |
| 界面和诊断 | [IPC 客户端](../../apps/macos/Sources/LoomLocalAppCore/LocalIPCClient.swift)、服务探测使用相同旧 socket；[界面存储](../../apps/macos/Sources/LoomLocalAppCore/LocalProductStore.swift)默认写 `chat-sessions.json`、`mission-presentations.json`；[导出器](../../apps/macos/Sources/LoomLocalAppCore/LocalDiagnosticBundle.swift)与[后台](../../cmd/loomd/operational_diagnostics.go)访问 `diagnostics/` | 客户端、探测、界面数据、日志及导出统一使用新根；不能把旧 socket 或旧日志当作新产品的健康和诊断证据 |
| 独立配置/激活入口 | [生产路由装配](../../cmd/loomd/product_composition_work.go)另选 `Application Support/Loom` 与 `LaunchAgents`；[配置写入](../../internal/production/config.go)使用 `daemon.json`，生成另一个 Label `com.loom.local.daemon`，传入 `run/loomd.db`。[传统安装器](../../scripts/install-loom-local-product.sh)另写 `bin/loom{,d}`、`Loom.command` | 首个隔离包不开放旧激活写入口，不投放旧 CLI/启动器；若以后交付，先设计独立配置、标识和明确的单一服务所有权 |
| 凭证及加密数据 | [Vault 路径派生](../../cmd/loomd/product_credential_vault.go)使用 `state/credential-vault.db`、`private/vault.key` 及待轮转文件。历史 [Keychain 服务名](../../internal/credentials/keychain_darwin.go)为 `com.earendilworks.loom.provider`，不是普通运行后端 | 只在新根创建新 Vault/密钥；不复用旧密钥、Keychain 服务或遗留发现。加密格式、协议角色、模块名与来源许可保留 |

两个易漏的接线：`LocalServiceProcessHost.start` 会先检查 socket 是否可达，**成功连接不是产品身份**；`expandLocalAppServiceArgs` 仍会检查 `~/.cc-switch/cc-switch.db` 是否存在以提供导入候选（不等于自动导入）。因此只改 plist、只换 socket 或只换数据库根，都不能提供并存保证。

## Decision（TARGET）

1. **同一个产品身份**决定包、服务和私有数据根。候选 ID 评审后冻结；构建时将同一声明落到 plist、Swift 与 Go（或用同一清单校验三者）。签名/目录安装/服务注册另行授权。保留包内 helper 名、现有 IPC、Vault AAD/HKDF 域及 `.loomvault` 格式，不全局替换历史标识。
2. **随包服务、前台回退、客户端同时切换**。固定绑定匹配的安装身份、当前用户与同一规范化根；目录、锁和跨产品重叠检查失败就停止。在连接前检查 socket 路径字节上限；不退到 Loom、公共临时目录或自动连接用户环境指定的旧服务。就绪需新根上的 IPC 读探针核对产品身份与协议兼容性；这用于辨认串连，不替代现有认证或声称同账户内强隔离。测试仍可显式注入虚构 home/socket，默认发行路径不借此绕过检查。
3. **旧文件和旧进程不归新安装器管理**。只在验证归属的 ZoriDesk 安装目标暂存、验证、替换或回退，未知已有目标、符号链接和身份冲突拒绝。卸载仅取消本产品注册和移除经验证的包，默认保留用户数据；禁止停用其他服务。新 daemon 不启用仍能写到旧 Loom 路径的独立激活路由，不能只改 `SMAppService` 标签就视为安全。
4. **首个隔离包为空数据启动**。不寻找或复制 Loom、CC Switch 或 Keychain；凭证需用户重新输入到新 Vault。加密对话/Vault、授权、活动任务、会话句柄以及执行历史不是首批迁移对象。普通启动不得触发跨产品导入，也不得改写旧库。旧安装保留为独立读取途径，不通过共享数据库实现回退。

### 后续显式导入设计

- **预览**：只读取用户选择的、版本可识别的导出，先审核允许导入的非授权设置类型；不打开运行中的旧数据库、不复制 WAL/密钥/凭证、不扫旧根。生成范围、来源版本、目标冲突和摘要，不将秘密或全路径写入状态记录。类型不明确时拒绝，不将旧授权解释成新授权。
- **确认并提交**：用户明确确认绑定到预览摘要的选择；持目标锁，复检源摘要、目标版本和归属。冲突默认不覆盖，显示可跳过项；先保存可验证的 ZoriDesk 目标备份和预备回执，在本产品的私有暂存区校验再提交。回执绑定来源、选项、目标身份与代次；提交与完成标记须可一起判定，只有匹配的完成回执才使重放成为无操作。
- **失败/回退**：预览变化、备份失败或校验失败均零提交；提交中断须能用回执和备份判定继续或恢复，失败提示不泄露路径、原始记录或凭证。保留旧源不变，只回退新目标；不能把新版数据直接交给不兼容的旧二进制。基于现有 Vault 格式的显式恢复若要支持，另行评审密钥与加密会话边界，不靠复制旧目录。

### 实现顺序与验收

先评审此方案。第一片只给 Swift/Go 增加可注入虚构 home 的身份/路径解析与交叉夹具，先加回归，不改默认运行或真实配置。第二片才成组接入构建、安装、服务、客户端和数据根；不能发布一半切换的包。干净并存验证完成后再评审独立导入范围。不跳到 Docker 或发布。

| 场景 | 实施后必须证明（均先用夹具） |
| --- | --- |
| 干净与已有 Loom | 同一虚构 home 下旧包/配置/库/锁/日志存在；新旧实例的受管数据各在本产品根。前台与注册路径对新身份一致；只提供旧 socket 时不能进入已连接。长路径、重叠目录、非预期链接或旧标签拒绝，旧哨兵内容和模式不变 |
| 重复安装与失败 | 目标相同且归属匹配才升级；注入暂存、提交和注册失败，复核旧包/旧服务未操作，新包回退且数据保留。重复执行不注册第二份，卸载不动 Loom |
| 预览、冲突与重试 | 预览零写入，变动源/不支持版本拒绝；确认后冲突默认保留目标。重放同一收据不重复导入。注入提交前后失败，验证目标备份恢复、旧源不变且无凭证流转 |
| 核验方式 | Go 普通/race 跑私有临时路径、故障和回执；macOS 核对构建、端点与包标识；真实注册/签名、已有旧安装与用户数据的验证不能用模拟结果替代，需获准的隔离环境 |

**本轮证据（CURRENT）**：定向执行 8 个现有 daemon 路径测试，普通/race 均通过、无跳过。`TestExpandLocalAppServiceArgsUsesStableUserPathsAndInstalledRuntime`、`TestExpandLocalAppServiceArgsCarriesExistingCredentialImportSource` 和 `TestQ11ExpandLocalAppServiceArgsWithoutPi` 使用夹具确认当前参数来源；另外验证软链拒绝、显式导入源校验与固定启动参数。两份 plist 通过语法检查。未运行安装、真实 daemon/GUI、注册、签名、迁移或凭证读写；这不是 TARGET 验收。关联基础：[随包生命周期](0016-bundled-local-service-lifecycle.md)、[Vault 边界](0020-loom-owned-credential-vault.md)。

## Alternatives Considered

- **仅改包名/Bundle ID**：改动少，但探测、恢复和配置写入仍可能串到 Loom；拒绝。
- **共享旧根或第一次启动复制整个目录**：看似省迁移步骤，却会共享锁/授权、复制敏感材料或得到不一致的库，失去独立回退；拒绝。

## Consequences

- 两套安装可独立交付和回退，代价是要一起检查各启动入口、首次需重新设置且不会自动带入历史。
- 安装器事务和 Vault 恢复原有约束不变；兼容性需要真实隔离证据。用户名路径长度、身份漂移、两个服务管理入口是实现回归门槛，不宣称任一问题已修复。
