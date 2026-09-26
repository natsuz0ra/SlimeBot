<p align="center">
  <img src="assets/title.png" alt="SlimeBot Logo" width="420" />
  <br /><br />
  <a href="README.md">English</a> | <strong>简体中文</strong>
</p>

# SlimeBot

个人练手的 Agent Demo，目标是搭建可扩展的 AI 会话应用雏形。采用 **Go** 后端、**Vue 3** Web 前端，以及 **React + Ink** 终端 CLI。

## 当前支持功能

- 会话列表、实时流式回复、多模态消息、会话标题自动生成
- Agent 多轮 tool call、审批模式、沙盒策略、命令/文件工具、网络请求、网络搜索和待办事项
- 桌面 Computer Use：让 SlimeBot 查看并操作电脑上的应用，支持 macOS、Windows 和 Linux，敏感操作需经过审批
- 规划模式、流式思考、上下文压缩、MCP 配置、Skills 与 AGENTS.md 指令
- Web UI、CLI TUI 与 Telegram 集成
- Web 会话可在首次对话前选择工作目录，侧边栏按项目归类；旧会话归入“未分类”，消息平台会话按平台归类

## 桌面应用

前往[最新版本下载页](https://github.com/natsuz0ra/SlimeBot/releases/latest)获取 macOS 或 Windows 桌面应用。打开后无需登录即可开始对话；关闭窗口后仍会在后台运行，可以从系统托盘退出。Windows 安装时可自行选择是否创建桌面快捷方式。

如果桌面应用与 Web 服务共用同一份数据，请不要同时运行，以免消息和计划任务重复执行。

## 从 Release 安装

用一条命令安装最新 Release：

macOS / Linux：

```bash
curl -fsSL https://github.com/natsuz0ra/SlimeBot/releases/latest/download/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://github.com/natsuz0ra/SlimeBot/releases/latest/download/install.ps1 | iex
```

也可以从项目 Release 下载适合当前系统的压缩包，解压后在解压目录执行 `./install.sh` 或 `.\install.ps1`。

安装脚本会在需要时自动下载匹配的压缩包，把 SlimeBot 放到用户本地目录，并创建命令入口。如果终端找不到 `slimebot`，请把安装脚本输出的 shim 目录加入 `PATH`。

启动 Web 服务前，请编辑 `~/.slimebot/config.cfg`，设置一个强随机的 `JWT_SECRET`。

## 更新

SlimeBot 可以通过已安装的命令检查 GitHub Release 并执行更新：

```bash
slimebot update --check          # 检查最新稳定 Release
slimebot update                  # 更新到最新稳定 Release
slimebot update --version v1.26.3
```

更新源沿用安装逻辑：优先读取 `SLIMEBOT_REPO`，否则使用 `natsuz0ra/SlimeBot`。Web 用户可在 **设置 -> 关于** 打开更新中心，CLI TUI 用户可输入 `/update`。

如果当前构建版本是 `dev`、空值，或无法解析为版本号，默认会禁用一键更新。确认要安装某个指定 Release 时，请使用 `slimebot update --version vX.Y.Z`。

## 卸载

用一条命令运行最新 Release 中的卸载脚本，或在解压后的 Release 目录、已安装目录中运行卸载脚本。

macOS / Linux：

```bash
curl -fsSL https://github.com/natsuz0ra/SlimeBot/releases/latest/download/uninstall.sh | sh
./uninstall.sh
```

Windows PowerShell：

```powershell
irm https://github.com/natsuz0ra/SlimeBot/releases/latest/download/uninstall.ps1 | iex
.\uninstall.ps1
```

卸载脚本会停止并卸载用户服务，删除程序安装目录和命令入口。删除 `~/.slimebot` 用户数据前会询问确认。使用 `--purge` / `-Purge` 可非交互删除用户数据，使用 `--yes` / `-Yes` 可非交互卸载但保留用户数据。

远程执行并传入参数：

```bash
curl -fsSL https://github.com/natsuz0ra/SlimeBot/releases/latest/download/uninstall.sh | sh -s -- --yes
curl -fsSL https://github.com/natsuz0ra/SlimeBot/releases/latest/download/uninstall.sh | sh -s -- --purge
```

```powershell
& ([scriptblock]::Create((irm https://github.com/natsuz0ra/SlimeBot/releases/latest/download/uninstall.ps1))) -Yes
& ([scriptblock]::Create((irm https://github.com/natsuz0ra/SlimeBot/releases/latest/download/uninstall.ps1))) -Purge
```

## 常用命令

```bash
slimebot                         # 启动 CLI TUI
slimebot cli                     # 显式启动 CLI TUI
slimebot server                  # 前台启动 Web 服务
slimebot service install         # 安装 Web 用户服务
slimebot service start           # 启动 Web 用户服务
slimebot service stop            # 停止 Web 用户服务
slimebot service restart         # 重启 Web 用户服务
slimebot service status          # 查看 Web 用户服务状态
slimebot service uninstall       # 卸载 Web 用户服务
slimebot update --check          # 检查更新
slimebot update                  # 应用最新稳定更新
slimebot version                 # 输出版本信息
slimebot help                    # 显示命令帮助
```

Web 默认端口为 **6247**。服务启动后访问 `http://localhost:6247`。

服务命令默认安装当前用户服务。macOS 使用 `~/Library/LaunchAgents`；Linux systemd 使用 `systemctl --user`，需要可用的用户服务会话。

macOS 服务 label 为 `com.natsuzora.slimebot`，plist 位于 `~/Library/LaunchAgents/com.natsuzora.slimebot.plist`，日志位于 `~/.slimebot/log/service.out.log` 和 `~/.slimebot/log/service.err.log`。如果 `slimebot service start` 提示旧版 `slimebot` 服务仍在 launchd 中，先运行 `launchctl bootout gui/$(id -u)/slimebot`；如果旧服务来自系统级 LaunchDaemon，再运行 `sudo launchctl bootout system /Library/LaunchDaemons/slimebot.plist`，然后重新执行 `slimebot service start`。

首次 Web 登录时，若数据库中尚无用户，会种子默认账号：用户名 **`admin`**，密码 **`admin`**。请立即修改。

## UI 预览

以下截图使用示例数据。

### 登录页

![登录页预览](assets/login-v1.33.0.png)

### 新对话与工作目录

![新对话与工作目录](assets/home-v1.33.0.png)

### 会话与项目侧边栏

![会话与项目侧边栏](assets/chat-v1.33.0.png)

## 功能状态与待办

**已完成：** Web/CLI 会话、WebSocket 流式回复、Agent 工具、桌面 Computer Use、审批、沙盒约束、规划模式、思考控制、子代理、MCP、Skills、AGENTS.md 指令、SQLite 摘要、Telegram、多模态与 JWT 认证。

**待完成：** 更多消息平台接入，如 Discord、Slack 等。

## 许可

本项目以 [MIT 许可证](LICENSE) 授权。
