# SlimeBot 1.32.0 桌面应用技术方案

状态：1.32.0 实施方案及预览版实现说明。基线：`origin/main` 的 `a0f4b203cc6c8bcb330864f1721c4d97e66f253c`。目标平台为 macOS 与 Windows；Linux 在后续版本验证。

## 1. 目标与边界

用户双击安装包即可启动 SlimeBot，在独立窗口使用现有 Vue 会话界面、Go Agent 能力、审批、Skills、MCP、Computer Use 和计划任务。安装包包含所需 Go 程序和前端资源，不要求用户先安装 Go、Node.js、命令行工具或后台服务。窗口关闭后任务是否继续、怎样彻底退出、出错时如何恢复，都必须有明确的桌面行为。

本版本实现单实例、窗口与托盘、受控启动和关闭、安装包、错误恢复和更新检查。多窗口工作区、第三方插件市场、云同步、数据迁移向导与深度原生化编辑器不列入首版。现有 Web、CLI 和服务模式继续使用同一业务实现。

“像官方 DeepSeek Harness Desktop”主要指完整 Web 工作区被桌面壳承载、后台进程受壳管理、任务在窗口隐藏后继续、桌面安装与更新有独立生命周期。其官方桌面实现是 Electron 包裹完整 Web 应用，并通过子进程运行 Host；这是体验和边界的参考，不表示复用它的代码或插件体系。[官方桌面 README](https://github.com/deepseek-ai/deepseek-harness/blob/master/apps/desktop/README.md)

## 2. 仓库现状与直接影响

| 现状 | 代码位置 | 桌面端影响 |
| --- | --- | --- |
| Go `server` 创建完整业务服务，内嵌 `web/dist` 并提供 REST 与 `/ws/chat` | `cmd/server/main.go`、`internal/app/app.go`、`internal/server/router/router.go`、`web/embed.go` | 可复用同一后端和 Vue 页面；不需要重写 UI 或 Agent 核心 |
| `server` 监听 `:SERVER_PORT`，默认 6247；`/health` 不鉴权 | `internal/app/app.go`、`internal/config/config.go`、`internal/server/router/router.go` | 桌面模式须绑定回环地址和随机端口，不能把本机 Agent 接口暴露到局域网 |
| 前端 API 使用同源相对路径，WebSocket 根据 `location.host` 生成 URL | `frontend/src/api/client.ts`、`frontend/src/api/chatSocket.ts` | 窗口加载受控的本地 Go 地址时，可复用现有前端请求链路 |
| 首次建库创建默认管理员；JWT 在前端 `localStorage`，WebSocket token 放在 URL 查询参数 | `internal/app/core.go`、`internal/server/middleware/auth_jwt.go`、`frontend/src/utils/authStorage.ts`、`frontend/src/api/chatSocket.ts` | 桌面首启必须移除默认口令风险，并避免凭据进入 URL、访问日志或崩溃记录 |
| WebSocket 当前接受任意 Origin | `internal/server/ws/ws_controller.go` | 需要对桌面来源和请求做校验，不能把“只监听 localhost”当成完整保护 |
| `server` 启动调度器，关闭进程后计划任务停止 | `internal/app/app.go` | 关窗隐藏与真正退出要区别处理，并提示计划任务影响 |
| 当前更新器下载并替换命令行发行包 | `internal/updater/`、`internal/server/controller/update.go` | 桌面安装包须有独立更新策略，桌面页面不能调用旧的替换接口 |
| Release 使用 Linux runner 交叉编译 Go 并制作压缩包 | `scripts/package-release.sh`、`.github/workflows/release.yml` | 桌面签名、macOS 公证和 Windows 安装包要在目标系统的 CI job 构建 |

桌面模式与现有服务模式是不同的进程入口，但共用 `internal/app`、路由、Vue 前端和数据模型。避免在 Electron 主进程复制聊天、认证或设置逻辑。

## 3. 技术选型

| 方案 | 适配度 | 主要代价 |
| --- | --- | --- |
| Electron 壳 + 打包的 Go 子进程（推荐） | 官方参考路径相近；已有 npm/TypeScript；前端仍由现有 Go HTTP 服务器提供，REST/WebSocket/静态资源无需协议迁移 | 安装包较大，需维护 Chromium/Node 安全更新和签名流程 |
| Tauri 2 壳 + Go sidecar | 安装包可较小，也能保留 Go 服务 | 增加 Rust 工具链及平台 WebView 差异；子进程和安装签名仍要处理 |
| Wails 壳 + Go 进程内服务 | Go 开发体验一致 | 现有同源 HTTP/WebSocket 页面仍需适配加载方式；平台 WebView 差异和原生生命周期同样存在 |

首版建议 Electron。理由是对现有 Go HTTP + Vue 协议的改动最少，桌面层只做进程、窗口、文件对话框和安装更新；待测得包体、内存与维护成本后再评估是否迁移。Electron 官方安全清单要求隔离渲染器、关闭 Node 集成、限制导航与 IPC；这些是实施门槛。[Electron 安全指南](https://www.electronjs.org/docs/latest/tutorial/security)

### 3.1 进程与目录

```text
SlimeBot Desktop（Electron 主进程，单实例）
  ├─ BrowserWindow：只加载本实例本地 Go 服务的 Vue 页面
  ├─ preload：只暴露更新检查、进度和安装操作
  └─ slimebot desktop-host（打包的 Go 子进程）
       ├─ 127.0.0.1:系统分配端口：静态页面、REST、WebSocket
       ├─ 内部服务：会话、工具、调度器、数据库
       └─ 与 Web/CLI 共用配置、数据库和日志
```

`desktop/` 管理 Electron 主进程、preload、图标与安装配置；Go 入口添加 `desktop-host` 子命令。`desktop-host` 运行完整业务服务，包括调度器和已配置的 Telegram。它与 Web/CLI 使用同一 `SLIMEBOT_HOME` 规则：默认从 `~/.slimebot/config.cfg` 读取配置，可通过环境变量指定其他配置目录；数据库、Skills 和上传路径沿用配置值，日志写入该配置目录。Electron 的平台 `userData` 保存应用自身状态与缓存。不要让桌面版和 Web 服务同时使用相同的数据路径，否则消息平台和计划任务可能重复运行。

### 3.2 启动协议

1. Electron 在访问用户数据前取得单实例锁；第二次启动仅唤醒已有窗口。[Electron 单实例 API](https://www.electronjs.org/docs/latest/api/app#apprequestsingleinstancelockadditionaldata)
2. 主进程启动打包的 Go 可执行文件，并保留现有 `SLIMEBOT_HOME` 与数据路径环境变量。启动随机密钥经 stdin 管道传递；Go 按常规规则读取配置，桌面端仅覆盖访问凭据和前端来源。工具执行继续走现有审批与沙盒。
3. Go 只监听 `127.0.0.1:0`，在监听成功后通过 stdout 管道返回地址和协议版本。主进程验证地址和版本后加载确切的本地 URL；启动超时、非零退出或版本不匹配时显示原生重试/退出对话框。
4. Electron 在同一应用会话中，仅对指向该地址的 API 与 WebSocket 请求注入 `Authorization` 头。Go 使用常量时间比较验证随机密钥；网页代码不能读取它。
5. Electron 退出时关闭 Go 的 stdin，等待优雅退出；超时再终止进程。后端异常退出时隐藏失效窗口并提供重试。

开发模式沿用 Vite 热更新；正式包只运行内嵌的 `web/dist`。可执行文件、`ripgrep` 和 Computer Use 所需平台辅助资源放在可执行的 unpacked resources 中，路径解析从应用资源目录开始，不能依赖用户的当前工作目录或系统 PATH。

## 4. 本地安全和首启

**本地监听不等于认证。** Go 桌面服务只绑定 `127.0.0.1`，仍要求每个 API 和 WebSocket 请求有认证。主进程每次启动生成 32 字节随机凭据，通过私有管道交给 Go，随后仅对本应用本地地址的请求注入认证头。桌面前端不使用 `localStorage` 中的 JWT，也不把凭据放入 WebSocket URL。preload 只暴露固定的更新方法，IPC 在主进程验证发送窗口和来源。

桌面采用用户指定的本机单用户免登录语义：不种子 `admin/admin`，不展示登录、改密或登出入口，Go 不注册登录/改密 API。脚本安装的 Web 服务继续使用账号密码。桌面与服务共用数据库；桌面访问凭据仅在本次桌面进程内存中设置，不进入命令行参数或 URL。

Electron 窗口设置 `nodeIntegration: false`、`contextIsolation: true`、`sandbox: true`；主进程验证 IPC 发送者，拦截导航和弹窗，将 HTTPS 外链交系统浏览器。Go 的 WebSocket 校验同源或明确配置的前端 Origin；桌面认证须来自回环地址且带启动凭据。后续安全加固包括针对嵌入页面配置更严格的 CSP 与本地服务 Host 检查。[Electron 安全指南](https://www.electronjs.org/docs/latest/tutorial/security)

Computer Use 的权限由操作系统授权窗口处理；桌面壳与 Go 工具仍遵循现有审批、沙盒和用户设置。安装包签名、公证、安全工具检测和平台权限描述需在各目标系统验证。

## 5. 窗口、任务与数据生命周期

默认关窗隐藏到系统托盘，Go 进程继续运行，消息平台和计划任务继续；托盘右键菜单提供“打开”和“退出 SlimeBot”。再次点击 Dock 或托盘可恢复窗口。显式退出会提示消息平台与后台任务停止，确认后关闭 Go 进程。首版的提示不逐项统计运行中的任务。[官方桌面 README](https://github.com/deepseek-ai/deepseek-harness/blob/master/apps/desktop/README.md#closing-the-window-and-quitting)

数据、配置、Skills 和上传按 Web/CLI 相同的路径规则读取；Electron 自己的缓存位于平台 `userData`。旧桌面预览版在 `userData/data` 中创建的数据不自动迁移。

首版沿用现有 Vue 文件交互和 Go 的路径授权策略，不在 Electron 重写文件浏览器。

## 6. 更新与发布

桌面版本与后端、前端保持同一 `1.32.0` 版本；安装包是一个整体。桌面模式不注册旧的 `/api/update/apply`。未签名预览版在设置页检查 GitHub Release 新版本并打开安装页，用户手动下载和覆盖安装。macOS 仅发布 DMG；Squirrel.Mac 自动更新同时要求代码签名和 ZIP 更新包，因此 macOS 应用内自动安装保持关闭，即使日后增加签名也须先恢复 ZIP 目标与发布。Windows 在配置签名凭据后可使用现有 `electron-updater` 桥接。[Electron 代码签名说明](https://www.electronjs.org/docs/latest/tutorial/code-signing) · [electron-builder 自动更新说明](https://www.electron.build/docs/features/auto-update/)

`desktop-preview.yml` 在开发分支构建 `SlimeBot-1.32.0-macos-universal.dmg` 与 `SlimeBot-1.32.0-windows-x64.exe`，并在以提交号命名的 GitHub 预发布版本中分别提供原文件直链；文件名遵循“应用名-版本-平台-目标架构”。稳定 tag 的现有 Release job 保持命令行包发布，随后 macOS/Windows job 将两个安装包作为独立文件追加到同一 GitHub Release。桌面应用图标与托盘图标均由深紫色 `frontend/public/slime-icon.svg` 生成。当前缺少 Developer ID、公证和 Windows 证书，工件为未签名预览包；首次运行可能需要用户按系统提示明确放行。待凭据具备后，CI 使用 Secrets 签名、公证，并分别在干净机器验证安装与升级。

## 7. 分阶段实施与验收

| 阶段 | 工作 | 可验收结果 |
| --- | --- | --- |
| A. 安全启动基础 | Go `desktop-host`、随机端口/回环监听、ready 管道、共用配置目录、无密码本机会话 | 双击启动；读取现有配置与会话；局域网无法访问；同机其他进程无法通过猜端口取得 Agent 权限 |
| B. 桌面壳 | Electron 单实例、窗口/托盘、最小 preload、子进程停止与恢复 | 关窗任务继续；显式退出停止后台；后端崩溃可重试 |
| C. 桌面体验 | 桌面更新入口、退出影响提示、项目 Logo 图标 | 用户无需终端配置和聊天；旧 Web/CLI 更新器不会替换桌面安装 |
| D. 预览发行 | macOS/Windows CI、未签名安装包、预览更新检查 | 两平台可下载安装；稳定 tag 可追加桌面工件；应用内自动安装待签名后验证 |

重点测试：REST 与 WebSocket 的随机凭据校验；窗口隐藏/恢复后后台任务继续；端口被占用与后端异常退出；重复启动；网络断开；审批等待时退出；macOS/Windows 首次安装和覆盖安装。签名、公证与自动升级测试在凭据配置后进行。

## 8. 后续里程碑

取得签名凭据后配置 macOS Developer ID、公证和 Windows 签名，在两平台完成自动更新的安装、回滚与后台任务中断验证。另行评估数据迁移向导、运行任务清单和 Linux 安装包。
