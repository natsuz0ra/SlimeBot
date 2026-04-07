# SlimeBot — Agent Guide

AI 会话 Agent 应用：Go 后端 + Vue 3 前端，生产环境 Go 二进制嵌入前端静态资源。

## Commands

```bash
# 安装依赖
npm install && npm install --prefix frontend

# 开发（concurrently 启动 Go 后端 + Vite，端口 8080 / 5173）
npm run dev

# 构建（前端 → Go 二进制，输出 slimebot 可执行文件）
npm run build

# 仅构建前端
npm run build:frontend

# 仅运行后端（需先构建前端或不需要前端时）
go run ./cmd/server/main.go

# Go 测试
go test ./...

# 运行单包测试
go test ./internal/services/chat/...

# 清理构建产物
make clean
```

## Architecture

- **Module name**: `slimebot`（`go.mod`）
- **Go**: 1.26, **Router**: chi v5, **ORM**: GORM + SQLite, **WebSocket**: gorilla/websocket
- **前端**: Vue 3 + TypeScript + Vite 7 + Pinia + Tailwind CSS 4

### Entry Points

- `cmd/server/main.go` — HTTP/WebSocket 服务

### Backend Layout (`internal/`)

| 目录 | 职责 |
|---|---|
| `app/` | 应用引导、依赖组装、生命周期（`New()` + `Run()`） |
| `server/` | HTTP 路由、控制器、中间件、WebSocket handler |
| `services/` | 业务逻辑层（auth, chat, config, embedding, memory, openai, session, settings, skill） |
| `repositories/` | 数据访问层（GORM/SQLite） |
| `domain/` | 接口定义与模型 |
| `tools/` | 内置工具（exec, http_request, web_search） |
| `platforms/` | 消息平台接入（Telegram） |
| `mcp/` | MCP 协议集成 |
| `runtime/` | 环境配置加载、路径管理 |
| `config/` | 配置结构体与解析 |
| `constants/` | 常量 |

### Frontend Layout (`frontend/src/`)

`api/`, `components/`, `composables/`, `pages/`, `stores/`, `styles/`, `types/`, `utils/`

## Key Conventions

- **工具注册**：在 `internal/tools/` 新建文件，实现 `Tool` 接口并在 `init()` 中调用 `Register()`。服务端通过 `_ "slimebot/internal/tools"` 的 blank import 触发自动注册。
- **go:embed**：`web/embed.go` 嵌入 `web/dist/`（前端构建产物），`prompts/embed.go` 嵌入 `system_prompt.md`。构建 Go 前必须先构建前端（`web/dist/` 需存在）。
- **前端构建输出到 `web/dist/`**（非 `frontend/dist/`），由 `vite.config.ts` 中 `outDir` 控制。
- **Vite 代理**：开发时 `/api` → `http://localhost:8080`，`/ws` → `ws://localhost:8080`。

## Config

- 运行时配置文件：`~/.slimebot/.env`（首次启动自动生成）
- `JWT_SECRET` 必填，未配置将启动失败
- 前端配置：`frontend/.env`（`VITE_API_BASE_URL`, `VITE_WS_URL`）
- 嵌入向量化依赖 ONNX Runtime + bge-m3 模型，首次启动自动下载到 `~/.slimebot/onnx/`
- 记忆检索：优先向量检索，失败自动降级为关键词检索

## Build Gotchas

- Go 使用 CGO（SQLite 驱动 `glebarez/sqlite` 需要），确保环境有 C 编译器
- Docker 构建中安装了 `gcc libc6-dev` 并设置 `CGO_ENABLED=1`
- 生产构建顺序：`npm run build:frontend` → `go build -o slimebot ./cmd/server`
- Windows 下构建产物为 `slimebot.exe`

## Runtime Data

所有运行时数据默认在 `~/.slimebot/`：`.env`、SQLite 数据库、Chroma 向量库、Skills、ONNX 模型文件。
