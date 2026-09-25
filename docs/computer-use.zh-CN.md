# 桌面 Computer Use

SlimeBot 提供 `computer__observe`、`computer__click`、`computer__type`、`computer__key` 和 `computer__scroll`。工具操作的是 **SlimeBot 服务进程所在机器的交互式桌面**，并非聊天用户自己的远程桌面。当前没有浏览器专用的 DOM 或 Playwright 路径。

## 工作方式

`observe` 默认读取前台应用的系统辅助功能树，并给每个元素返回临时 `ref`。当辅助功能不可用或没有可用元素时，自动截取主屏幕并把图片交给支持图像输入的模型。也可传 `mode=screenshot` 强制截图，或传 `mode=accessibility` 只读取辅助功能树。

操作需要最近一次 `observe` 返回的 `snapshot_id`。辅助功能操作使用元素 `ref`；截图操作使用返回图片中的 `x,y` 像素坐标。每个快照只允许执行一次操作，且两分钟后过期；操作完成后应重新观察。截图最长边压到 2048 像素，坐标按压缩后的图片换算。当前以单显示器桌面和前台窗口为验证范围；macOS、Windows 的截图取主显示器。

## 系统要求

| 系统 | 辅助功能 | 截图与坐标输入 |
| --- | --- | --- |
| macOS | AXUIElement；需给启动 SlimeBot 的应用（例如 iTerm2）授予“辅助功能”权限 | 系统 `screencapture` 和 Quartz 鼠标、键盘事件；需允许“录屏与系统录音”。授权录屏后须退出并重新打开启动应用。 |
| Windows | UI Automation；SlimeBot 必须运行在有交互式桌面的用户会话中 | PowerShell、.NET 截图与 Windows 输入 API；后台服务会话不能操作用户桌面。 |
| Linux X11 | Python `pyatspi` / AT-SPI2，桌面应用须暴露辅助功能节点 | 安装 `xdotool`，以及 `gnome-screenshot`、`scrot` 或 ImageMagick `import` 中至少一种。 |
| Linux Wayland | Python `pyatspi` / AT-SPI2，桌面应用须暴露辅助功能节点 | 安装 `ydotool` 并运行 `ydotoold`，以及 `grim`、KDE Spectacle 或 `gnome-screenshot` 中至少一种；截图工具必须受当前合成器支持。 |

Linux 需在同一图形会话中启动 SlimeBot，并使其能访问该会话的 AT-SPI D-Bus。Wayland 的截图与模拟输入取决于合成器及其授权策略；缺少工具或授权时，工具会返回具体错误。纯 SSH、无头部署和服务器的后台服务会话没有可操作的桌面。

## Wayland 实测范围

[GitHub Actions 实测](https://github.com/natsuz0ra/SlimeBot/actions/runs/36092777863)使用 Ubuntu 托管主机启动原生 Wayland 合成器，并用 GTK 窗口确认截图内容、`/dev/uinput` 点击及键盘输入是否真正送达。结果如下：

| Ubuntu | Sway | Labwc | GNOME Shell | KWin |
| --- | --- | --- | --- | --- |
| 24.04 | 截图、鼠标与键盘通过 | 截图、鼠标与键盘通过 | 截图通过；无头会话未收到 `/dev/uinput` 点击 | 截图通过；虚拟后端未收到 `/dev/uinput` 点击 |
| 26.04 | 截图、鼠标与键盘通过 | 截图、鼠标与键盘通过 | GTK 窗口已启动；`gnome-screenshot` 超时且 Shell 截图接口拒绝访问；未收到点击 | GTK 窗口已启动；虚拟 KWin 未提供 Spectacle 所需截图服务，未收到点击 |

GNOME/KWin 测试启动的是无头或虚拟合成器，缺少登录管理器提供的交互式 seat，因此未收到输入的结果不能直接推断普通桌面登录会话也会失败。GNOME/KDE 两个桌面的完整截图和输入链路仍需在实际登录的 Wayland 会话中复验。工作流中的 `desktop-probe` 可手动触发，失败时会上传截图和日志；正常推送持续运行 Sway/Labwc 的四种组合。

## 权限与限制

`computer` 属于需审批的敏感工具，遵循现有工具审批模式。屏幕及辅助功能内容会发送给当前配置的模型；请在可信桌面会话中使用。截图降级需要模型支持图像输入。辅助功能元素可能在观察后改变，工具会在操作前重新确认前台进程、元素路径、角色和名称；验证失败时需要重新观察。

在 macOS 上可执行 `SLIMEBOT_COMPUTER_LIVE_TEST=1 go test -count=1 ./internal/tools -run '^TestComputerObserveLive$' -v` 验证本机观察路径。执行 `SLIMEBOT_COMPUTER_ACTION_TEST=1 go test -count=1 ./internal/tools -run '^TestComputerActionLive$' -v` 会打开临时系统对话框，验证辅助功能输入与点击后自动关闭。执行 `SLIMEBOT_COMPUTER_SCREENSHOT_ACTION_TEST=1 go test -count=1 ./internal/tools -run '^TestComputerScreenshotActionLive$' -v` 会验证截图、图片坐标点击和向已聚焦输入框键入文字。跨平台 Go 编译可以在 macOS 上完成，但 Windows 与 Linux 的实际桌面权限、截图和输入仍需在相应图形会话中验证。
