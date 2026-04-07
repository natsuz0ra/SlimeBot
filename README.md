<p align="center">
  <img src="assets/title.png" alt="SlimeBot Logo" width="420" />
</p>

这是一个个人练手的 Agent Demo 项目，目标是实现可持续扩展的 AI 会话应用雏形。

## 1. 当前支持功能

- 会话与消息
  - 会话列表、创建、重命名、删除
  - 按会话拉取历史消息
  - 基于 WebSocket 的实时流式回复（start/chunk/done）
  - 会话标题自动生成与更新推送
  - 多模态能力
- 工具与 Agent
  - Agent 多轮 tool call 执行链路
  - 工具调用审批机制（高风险工具需前端确认）
  - 工具结果写入会话历史并支持详情查看
  - 内置工具：`exec`、`http_request`、`web_search`（Tavily）
- 记忆能力
  - 会话摘要自动更新
  - 长会话上下文压缩与最近消息回补
  - 跨会话记忆检索并注入提示词上下文
- 配置与扩展
  - LLM 配置管理（增/删/列）
  - MCP 配置管理（增/改/删/启停）与工具加载
  - Skills 上传安装、列表、删除与运行时激活
- 消息平台（当前支持 Telegram）
  - 消息平台配置管理（新增、更新、启用/停用）
  - 平台消息接入与回复
- CLI TUI
  - 独立 CLI 进程
  - 支持对话以及基本的配置等功能

## 2. UI 预览

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

### Cli

<img src="assets/cli.png" alt="cli" width="800" />

## 3. 架构与技术栈

### 架构说明

- 生产：Go 进程同时提供 REST/WebSocket 与嵌入的前端静态资源（`web/dist`）
- 开发：`npm run dev` 同时启动 Go 与 Vite，Vite 将 `/api`、`/ws` 代理到 `8080`
- 后端通过服务层访问模型接口，并将数据持久化到 SQLite（默认 `~/.slimebot/storage/data.db`）
- 记忆检索采用混合策略：优先向量检索，失败时自动回退关键词检索

### 技术栈

- 前端：Vue 3
- 后端：Go
- 记忆向量化：Go 原生 ONNX Runtime + 嵌入式 Chroma 存储 + bge-m3 模型

## 4. 如何启动

> 默认端口：后端 `8080`，Vite `5173`

在项目根目录：

```powershell
npm install
npm install --prefix frontend
npm run dev
```

首次启动会自动在 `~/.slimebot/.env` 生成配置文件（若缺失），并自动补齐新增配置项。

生产构建（根目录生成嵌入前端的 `slimebot` 可执行文件）：

```bash
npm run build
```

单独运行已构建的后端（仅提供 API + 静态页）：

```bash
go run ./cmd/server/main.go
```

运行 CLI TUI（本地终端模式）：

```bash
npm run cli
```

CLI 内置命令：

- `/new` 新建会话（懒创建，首次发送消息才真正建会话）
- `/session` 会话菜单（切换 / 删除）
- `/model` 模型菜单（切换全局默认模型）
- `/skills` 技能菜单（查看信息 / 删除）
- `/mcp` MCP 菜单（增删改查，内置多行编辑）
- `/help` 帮助

## 5. 数据与资源目录（默认）

所有运行时数据默认集中在 `~/.slimebot`：

```text
~/.slimebot/
  .env
  skills/
  storage/
    data.db
    chat_uploads/
    chroma/
  onnx/
    model.onnx
    model.onnx_data
    tokenizer.json
    runtime/
```

- `.env`：配置文件
- `storage/data.db`：SQLite 主数据库
- `storage/chroma`：Chroma 向量数据库持久化目录
- `skills`：Skills 存储目录
- `onnx/runtime`：ONNX Runtime 自动下载缓存目录
- `onnx`：bge-m3 模型文件目录

## 6. ONNX Runtime 与 bge-m3 自动下载

### ONNX Runtime

- 若未设置 `EMBEDDING_ORT_LIB_PATH`，启动时会根据当前系统自动下载 ONNX Runtime 共享库
- 默认下载/缓存目录：`~/.slimebot/onnx/runtime`
- 可通过 `EMBEDDING_ORT_VERSION`、`EMBEDDING_ORT_DOWNLOAD_BASE_URL` 覆盖版本与下载源

### bge-m3 模型

- 启动时会自动检查并补齐以下文件（缺哪个下哪个）：
  - `model.onnx`
  - `model.onnx_data`
  - `tokenizer.json`
- 默认路径：
  - `EMBEDDING_MODEL_PATH=~/.slimebot/onnx/model.onnx`
  - `EMBEDDING_TOKENIZER_PATH=~/.slimebot/onnx/tokenizer.json`
- 可通过 `EMBEDDING_MODEL_DOWNLOAD_BASE_URL` 覆盖下载源（默认 Hugging Face）

## 7. 配置文件（`~/.slimebot/.env`）

后端启动时读取 `~/.slimebot/.env`：

- `SERVER_PORT`：服务端口，默认 `8080`
- `DB_PATH`：SQLite 文件路径，默认 `~/.slimebot/storage/data.db`
- `SKILLS_ROOT`：Skills 根目录，默认 `~/.slimebot/skills`
- `CHAT_UPLOAD_ROOT`：聊天附件目录，默认 `~/.slimebot/storage/chat_uploads`
- `FRONTEND_ORIGIN`：与 Vite 联调时设为 `http://localhost:5173`；生产同源可留空
- `WEB_SEARCH_API_KEY`：Tavily 网络搜索 API Key
- `JWT_SECRET`：JWT 签名密钥（必填，未配置将启动失败）
- `JWT_EXPIRE`：JWT 过期时间（单位：分钟，默认 `21600` 即 15 天）
- `EMBEDDING_PROVIDER`：embedding 提供方式，默认 `onnx_go`（兼容 `onnx`）
- `EMBEDDING_MODEL_PATH`：ONNX 模型路径，默认 `~/.slimebot/onnx/model.onnx`
- `EMBEDDING_TOKENIZER_PATH`：tokenizer 路径（支持目录或 `tokenizer.json` 文件），默认 `~/.slimebot/onnx/tokenizer.json`
- `EMBEDDING_MODEL_DOWNLOAD_BASE_URL`：bge-m3 模型下载基地址，默认 `https://huggingface.co/BAAI/bge-m3/resolve/main/onnx`
- `EMBEDDING_ORT_VERSION`：自动下载 ONNX Runtime 的版本，默认 `1.24.1`
- `EMBEDDING_ORT_CACHE_DIR`：ONNX Runtime 缓存目录，默认 `~/.slimebot/onnx/runtime`
- `EMBEDDING_ORT_LIB_PATH`：本地 ONNX Runtime 共享库绝对/相对路径（设置后不自动下载）
- `EMBEDDING_ORT_DOWNLOAD_BASE_URL`：ONNX Runtime 下载基地址，默认 `https://github.com/microsoft/onnxruntime/releases/download`
- `EMBEDDING_TIMEOUT_MS`：embedding 超时毫秒数，默认 `30000`
- `CHROMA_PATH`：Chroma 持久化目录，默认 `~/.slimebot/storage/chroma`
- `CHROMA_COLLECTION`：向量集合名，默认 `session_memories`
- `MEMORY_VECTOR_TOPK`：向量检索返回条数，默认 `5`

示例：

```env
SERVER_PORT=8080
DB_PATH=~/.slimebot/storage/data.db
SKILLS_ROOT=~/.slimebot/skills
CHAT_UPLOAD_ROOT=~/.slimebot/storage/chat_uploads
WEB_SEARCH_API_KEY=YOUR_TAVILY_API_KEY
JWT_SECRET=CHANGE_ME_TO_A_RANDOM_SECRET
JWT_EXPIRE=21600

# FRONTEND_ORIGIN=http://localhost:5173

EMBEDDING_PROVIDER=onnx_go
EMBEDDING_MODEL_PATH=~/.slimebot/onnx/model.onnx
EMBEDDING_TOKENIZER_PATH=~/.slimebot/onnx/tokenizer.json
EMBEDDING_MODEL_DOWNLOAD_BASE_URL=https://huggingface.co/BAAI/bge-m3/resolve/main/onnx
EMBEDDING_ORT_VERSION=1.24.1
EMBEDDING_ORT_CACHE_DIR=~/.slimebot/onnx/runtime
EMBEDDING_ORT_LIB_PATH=
EMBEDDING_ORT_DOWNLOAD_BASE_URL=https://github.com/microsoft/onnxruntime/releases/download
EMBEDDING_TIMEOUT_MS=30000
CHROMA_PATH=~/.slimebot/storage/chroma
CHROMA_COLLECTION=session_memories
MEMORY_VECTOR_TOPK=5
```

### 前端配置：`frontend/.env`

- `VITE_API_BASE_URL`：后端 HTTP 地址（例如 `http://localhost:8080`）
- `VITE_WS_URL`：后端 WebSocket 地址（例如 `ws://localhost:8080`）

示例：

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

### 启用条件与降级行为

以下任一情况出现时，记忆能力会自动降级为关键词检索：

- `EMBEDDING_PROVIDER` 不是 `onnx_go`/`onnx`
- `EMBEDDING_MODEL_PATH` 或 `EMBEDDING_TOKENIZER_PATH` 缺失
- `CHROMA_PATH` 或 `CHROMA_COLLECTION` 缺失
- 向量库初始化失败（例如 Chroma 初始化失败）

## 8. 功能状态与待办

### 已完成

- 会话管理与消息流式回复（WebSocket）
- Agent 工具调用与审批流程
  - `exec`：系统命令行执行器
  - `http_request`：HTTP 请求器
  - `web_search`：基于 Tavily 的网络搜索器
- MCP 配置与工具执行能力
- Skills 包管理与运行时激活
- 会话持久化记忆与主动检索
- 消息平台基础能力（当前支持 Telegram）
- 多模态支持
- 记忆向量化存储与检索

### 待完成功能

- 更多消息平台接入（如Discord、Slack 等）
