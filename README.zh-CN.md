<p align="center">
  <img src="assets/title.png" alt="SlimeBot Logo" width="420" />
  <br /><br />
  <a href="README.md">English</a> | <strong>简体中文</strong>
</p>

# SlimeBot

个人练手的 Agent Demo，目标是搭建可扩展的 AI 会话应用雏形。采用 **Go** 后端、**Vue 3** Web 前端，以及 **React + Ink** 终端 CLI。

## 当前支持功能

- **会话与消息**
  - 会话列表、创建、重命名、删除
  - 按会话拉取历史消息
  - 基于 WebSocket 的实时流式回复（`start` / `chunk` / `done`，以及 `error`、工具调用与子代理相关事件）
  - 会话标题自动生成与更新推送
  - 多模态能力
- **工具与 Agent**
  - Agent 多轮 tool call 执行链路
  - 审批模式支持：**标准模式**（敏感工具需确认）与**自动执行**（直接执行）
  - 敏感内置工具需用户确认（当前为 `exec`），支持 Web、CLI、Telegram 等流程
  - 工具结果写入会话历史并支持详情查看
  - 内置工具：`exec`、`http_request`、`web_search`（Tavily）、**`run_subagent`**（子代理 / 嵌套 Agent）
  - **子代理：**主 Agent 可将独立子任务交给内层 Agent，内层使用**隔离上下文**（不携带父会话聊天记录）。仅支持**一层嵌套**（子代理内不能再调用 `run_subagent`）。子代理内的工具调用在 Web 与 CLI 中**嵌套展示**在父工具之下；历史记录持久化 `parentToolCallId`，刷新会话后层级仍可还原。
  - WebSocket 子代理流式事件：`subagent_start`、`subagent_chunk`、`subagent_done`（与 `tool_call_start` / `tool_call_result` 并存）
- **规划与思考控制**
  - 规划模式（Plan Mode）：先产出计划，再审批后执行
  - 计划生命周期：生成、同意/拒绝、修改并重生成、审批后执行
  - 思考等级控制（`off` / `low` / `medium` / `high`）
  - Web 与 CLI 均支持思考流式事件展示与时间线呈现
- **记忆能力**
  - 会话摘要自动更新
  - 长会话上下文压缩与最近消息回补
  - 跨会话检索：基于 `~/.slimebot/memory` 下的 Markdown 记忆文件，并建立全文索引供搜索与注入上下文
- **配置与扩展**
  - LLM 配置管理（增/删/列）
  - MCP 配置管理（增/改/删/启停）与工具加载
  - Skills 上传安装、列表、删除与运行时激活
- **消息平台**（当前支持 Telegram）
  - 消息平台配置管理（新增、更新、启用/停用）
  - 平台消息接入与回复
- **CLI TUI**
  - 独立 CLI（无头 Go 子进程 + Ink 终端界面），支持对话与基本配置
- **Web 界面**
  - 可选多语言（如中/英），基于 vue-i18n

## UI 预览

### 登录页

![登录页预览](assets/login.png)

### 主页

![主页预览](assets/home.png)

### 会话页

![会话页预览](assets/chat.png)

### 工具执行

![工具执行](assets/tool_exec.png)

### 消息平台（Telegram）

<img src="assets/tg_chat.jpg" alt="消息平台预览" width="220" />

### CLI

<img src="assets/cli.png" alt="CLI" width="800" />

## 架构与技术栈

- **生产**：Go 进程同时提供 REST/WebSocket，并通过 `go:embed` 嵌入 `web/dist` 静态资源，单一可执行文件交付。
- **开发**：`npm run dev` 同时启动 Go 与 Vite；Vite 将 `/api`、`/ws` 代理到 `8080`。
- **数据**：默认 SQLite，路径 `~/.slimebot/storage/data.db`。
- **记忆**：`~/.slimebot/memory` 下的 Markdown 记忆文件 + 本地全文索引，用于检索与写入提示词上下文。

**技术栈（概览）：** Go 后端 · Vue 3 Web 前端 · React + Ink CLI。

## 如何启动

默认端口：后端 **8080**，Vite **5173**。

在仓库根目录：

```bash
make deps
npm run dev
```

或手动安装依赖：

```bash
npm install
npm install --prefix frontend
npm run dev
```

首次启动会在缺失时创建 `~/.slimebot/.env`；后续若嵌入式模板新增键名，会按需追加到现有文件。

**首次登录（Web 服务模式）：** 若数据库中尚无用户，会种子一个默认账号（用户名 **`admin`**，密码 **`admin`**），并引导修改密码。除本机尝鲜外请尽快修改。

**生产构建**（生成嵌入前端的 `slimebot` 可执行文件）：

```bash
npm run build
# 或
make build
```

**仅运行后端**（需先完成前端构建以提供静态页）：

```bash
go run ./cmd/server/main.go
```

**CLI TUI：**

```bash
npm run cli
```

`make cli` 会安装 CLI 的 npm 依赖、构建 CLI（React + Ink）并生成 `slimebot-cli` 可执行文件（见 [Makefile](Makefile)）。

**测试：**

```bash
make test
# 或
go test ./...
```

**Docker：**

```bash
make docker-build
make docker-run
```

**Docker Compose：**

```bash
make compose-up
make compose-down
```

### CLI 内置命令

- `/new` 新建会话（懒创建，首次发送消息才真正建会话）
- `/session` 会话菜单（切换 / 删除）
- `/model` 模型菜单（切换全局默认模型）
- `/skills` 技能菜单（查看信息 / 删除）
- `/mcp` MCP 菜单（增删改查，内置多行编辑）
- `/mode` 切换审批模式（`standard` / `auto`）
- `/effort` 设置思考等级（`off` / `low` / `medium` / `high`）
- `/plan` 切换规划模式（`on` / `off`）
- `/help` 帮助

## 数据与资源目录（默认）

所有运行时数据默认集中在 `~/.slimebot`：

```text
~/.slimebot/
  .env
  skills/
  memory/
    MEMORY.md
    index.bleve/
    *.md
  storage/
    data.db
    chat_uploads/
```

- `.env`：配置文件
- `memory/`：Markdown 记忆条目、清单文件 `MEMORY.md`，以及 `index.bleve/` 下的全文索引数据
- `storage/data.db`：SQLite 主数据库
- `storage/chat_uploads`：聊天附件
- `skills`：Skills 存储目录

## 记忆存储（工作机制）

- 每条记忆为带 YAML frontmatter 的 Markdown 文件，根目录由 `MEMORY_DIR` 指定（默认 `~/.slimebot/memory`）。
- `memory/index.bleve/` 下为 Bleve 全文索引，用于搜索与跨会话召回。
- 服务启动时会异步重建索引（见 `Core.WarmupInBackground`）。

## 配置文件（`~/.slimebot/.env`）

后端会读取下列变量（括号内为默认值或说明）：

- `SERVER_PORT`：服务端口，默认 `8080`
- `DB_PATH`：SQLite 文件路径，默认 `~/.slimebot/storage/data.db`
- `SKILLS_ROOT`：Skills 根目录，默认 `~/.slimebot/skills`
- `CHAT_UPLOAD_ROOT`：聊天附件目录，默认 `~/.slimebot/storage/chat_uploads`
- `MEMORY_DIR`：记忆 Markdown 与索引根目录，默认 `~/.slimebot/memory`
- `FRONTEND_ORIGIN`：与 Vite 联调时设为 `http://localhost:5173`；生产同源可留空
- `WEB_SEARCH_API_KEY`：Tavily API Key，供 `web_search` 使用
- `JWT_SECRET`：**服务端模式必填**，未配置将启动失败（CLI 无头模式可自动生成）
- `JWT_EXPIRE`：JWT 过期时间（单位：分钟，默认 `21600` 即约 15 天）
- `approvalMode`（应用设置）：`standard` 或 `auto`
- `thinkingLevel`（应用设置）：`off` / `low` / `medium` / `high`

首次启动生成的 `.env` 与嵌入式模板一致，见 [internal/runtime/env.template](internal/runtime/env.template)。其他键可按需自行追加。

示例：

```env
SERVER_PORT=8080
DB_PATH=~/.slimebot/storage/data.db
SKILLS_ROOT=~/.slimebot/skills
CHAT_UPLOAD_ROOT=~/.slimebot/storage/chat_uploads
MEMORY_DIR=~/.slimebot/memory
WEB_SEARCH_API_KEY=YOUR_TAVILY_API_KEY
JWT_SECRET=CHANGE_ME_TO_A_RANDOM_SECRET
JWT_EXPIRE=21600

# FRONTEND_ORIGIN=http://localhost:5173
```

### 前端配置：`frontend/.env`

- `VITE_API_BASE_URL`：后端 HTTP 地址（例如 `http://localhost:8080`）
- `VITE_WS_URL`：后端 WebSocket 地址（例如 `ws://localhost:8080`）

示例：

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

## 功能状态与待办

### 已完成

- 会话管理与 WebSocket 流式回复（含错误、工具调用、子代理与思考事件）
- Agent 工具与审批（标准模式下 `exec` 需确认；支持可选自动审批模式）
- 规划模式：计划生成、同意/拒绝/修改流程，以及审批后执行
- 思考等级控制（`off` / `low` / `medium` / `high`）与流式思考展示
- 子代理 / 嵌套 Agent（`run_subagent`）、嵌套工具 UI，以及工具历史中的父子关联持久化
- MCP 与 Skills
- 基于文件与全文索引的持久化记忆及上下文注入
- Telegram 集成
- 多模态支持
- JWT 认证与默认管理员种子

### 待完成功能

- 更多消息平台接入（如 Discord、Slack 等）

## 许可

本项目以 [MIT 许可证](LICENSE) 授权。
