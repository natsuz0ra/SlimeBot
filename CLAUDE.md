# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SlimeBot 是一个个人 AI Agent Demo 项目，实现可扩展的 AI 会话应用雏形。后端为 Go，前端为 Vue 3，CLI 为 React+Ink (Node.js)。生产环境通过 `go:embed` 嵌入前端静态资源构建单一二进制文件。

## Build & Run Commands

```bash
# 安装依赖（根目录 + frontend + cli）
make deps

# 开发模式（concurrently 启动 Go 后端 + Vite 前端，Vite 代理 /api /ws 到 8080）
npm run dev

# 独立启动前端（Vite 开发服务器，端口 5173）
npm run --prefix frontend dev

# 独立启动后端（Go HTTP 服务器，端口 8080）
go run ./cmd/server/main.go

# 生产构建（前端先构建到 web/dist/，Go 再 embed 生成 slimebot 二进制）
make build
# 或
npm run build

# 前端独立构建（输出到 web/dist/）
npm run --prefix frontend build

# CLI 开发模式（自动构建 + 启动）
npm run cli

# CLI 独立构建
npm --prefix cli run build

# 运行测试
make test
# 或
go test ./...

# 运行单个包的测试
go test ./internal/services/chat/...
go test -v ./internal/services/chat/... -run TestSpecific

# 带 race 检测的测试
go test -race ./...

# 测试覆盖率
go test -cover ./...
go test -coverprofile=coverage.out ./...

# Docker
make docker-build
make docker-run
```

## Architecture

**分层架构**：`cmd/` → `internal/app/` → `internal/server/` → `internal/services/` → `internal/repositories/` → `internal/domain/`

- **`cmd/server/`** - HTTP 服务器入口。`_ "slimebot/internal/tools"` 空白导入触发 `init()` 注册内置工具
- **`cmd/cli/`** - CLI 入口，启动 headless HTTP 服务 + Node.js 子进程（React + Ink TUI）
- **`internal/app/`** - 应用引导层。`Core` 聚合所有服务，`App` 管理 HTTP 服务器与平台 worker 生命周期。`NewCore()` 中手动依赖注入，embedding/向量化服务异步预热
- **`internal/domain/`** - 领域模型（`models.go`）与接口定义。核心接口 `ChatStore` 在消费方定义，非实现方
- **`internal/services/`** - 业务逻辑层：chat（核心对话逻辑，含 Agent 多轮 tool call）、session、memory（摘要+跨会话检索）、embedding（BGE-M3 ONNX）、openai（API 封装）、skill（包管理与运行时）、auth、config
- **`internal/repositories/`** - GORM + SQLite 数据访问层，`Repository` 聚合所有子仓储。含向量检索仓储（Chroma）
- **`internal/server/`** - Chi 路由、控制器、WebSocket、中间件（JWT、限流 400/min 全局 30/min 登录）。支持 CLI token 旁路认证（`RouterConfig.CLIToken`）和 headless 模式
- **`internal/tools/`** - 内置工具通过 `init()` + `Register()` 注册到全局注册中心。当前内置：`exec`、`http_request`、`web_search`（Tavily）
- **`internal/mcp/`** - MCP 协议集成，支持 stdio/HTTP SSE 多传输，按配置缓存连接池，工具名自动去冲突
- **`internal/platforms/telegram/`** - 消息平台集成，`Dispatcher` 统一消息路由，`ApprovalBroker` 处理交互式审批
- **`frontend/`** - Vue 3 + TypeScript + Vite + Tailwind CSS 4 + Pinia。生产构建输出到 `web/dist/`（非 `frontend/dist/`），由 Go `web/` 包 embed
- **`cli/`** - React 19 + Ink TUI + TypeScript + tsup 构建，通过 WebSocket 与 headless Go 后端通信
- **`prompts/`** - `system_prompt.md` 通过 `go:embed` 注入到 chat 服务

**关键设计决策**：
- 接口在消费方定义，不在实现方
- 手动依赖注入（`app.NewCore()` 中组装）
- 工具注册：空白导入触发 `init()` → `Register()` 注册到全局 map，重名 panic
- 内存检索混合策略：优先向量检索（ONNX Runtime + BGE-M3 + Chroma），失败时回退关键词检索
- WebSocket 实时流式聊天回复（start/chunk/done 协议）
- Agent 支持多轮 tool call 链路，高风险工具需前端审批

## Frontend Architecture

Vue 3 前端采用组合式 API 和功能分层组织：

- **`src/components/`** - 按功能分组的 Vue 组件
  - `chat/` - 聊天相关组件（消息列表、输入框、工具调用卡片）
  - `settings/` - 设置页面组件（LLM/MCP/Skills/平台配置）
  - `home/` - 主页布局组件（侧边栏、头部、对话框）
  - `ui/` - 通用 UI 组件（图标、输入框、对话框、开关）
- **`src/composables/`** - 可复用组合式函数
  - `chat/` - 聊天相关逻辑（上下文管理）
  - `home/` - 主页交互逻辑（会话操作、模型选择、UI 状态）
  - `settings/` - 设置页逻辑（各配置项的 CRUD）
  - `useTheme.ts`, `useToast.ts`, `useLanguagePreference.ts` - 通用 composables
- **`src/stores/`** - Pinia 状态管理（`auth.ts`, `chat.ts`）
- **`src/api/`** - API 客户端（按领域分组：`auth.ts`, `chat.ts`, `chatSocket.ts`, `llm.ts`, `mcp.ts`, `skills.ts`）
- **`src/types/`** - TypeScript 类型定义（`chat.ts`, `settings.ts`）
- **`src/utils/`** - 工具函数（`markdown.ts`, `format.ts`, `authStorage.ts`, `replyBatchBuilder.ts`）

**前端构建特性**：
- Vite 将构建输出到 `web/dist/`（而非 `frontend/dist/`），由 Go `web/` 包 embed
- 开发时 Vite 代理 `/api` 和 `/ws` 到 Go 后端（`localhost:8080`）
- 支持 i18n（`vue-i18n`），语言切换通过 `useLanguagePreference.ts`

## WebSocket Protocol

聊天使用 WebSocket 实现实时流式回复。消息协议：

- **start**: `{ type: "start", messageId: string }` - 开始流式回复
- **chunk**: `{ type: "chunk", messageId: string, content: string }` - 内容块
- **done**: `{ type: "done", messageId: string, title?: string }` - 完成（可选会话标题）
- **error**: `{ type: "error", messageId: string, error: string }` - 错误

工具调用审批流程：
- 前端接收工具调用请求，用户确认后通过 WebSocket 发送批准响应
- 后端执行工具，结果通过流式回复返回

## Configuration

运行时配置通过 `~/.slimebot/.env` 管理，首次启动自动生成。关键必填项：`JWT_SECRET`。默认端口：后端 `8080`，Vite `5173`。前端环境变量在 `frontend/.env`（`VITE_API_BASE_URL`、`VITE_WS_URL`）。

## Code Conventions

- 不可变模式：始终创建新对象，不修改已有对象
- 错误处理：使用 `fmt.Errorf("context: %w", err)` 包装错误上下文
- 文件组织：小文件优先（200-400 行典型，800 行上限），按功能/领域组织
- Go 接口：保持小接口（1-3 个方法），接受接口返回结构体
- 表驱动测试
- Commit 格式：`<type>: <description>`（feat, fix, refactor, docs, test, chore, perf, ci）

## CLI Architecture

CLI 采用 React 19 + Ink（React 终端 UI）构建：

- **`cli/src/components/`** - Ink 组件（输入框、菜单、时间线、审批视图等）
- **`cli/src/index.tsx`** - CLI 入口，通过 WebSocket 与 headless Go 后端通信
- **`cli/src/app.tsx`** - 主应用组件，处理 CLI 生命周期

**CLI 内置命令**：
- `/new` - 新建会话（懒创建，首次发送消息才真正建会话）
- `/session` - 会话菜单（切换/删除）
- `/model` - 模型菜单（切换全局默认模型）
- `/skills` - 技能菜单（查看信息/删除）
- `/mcp` - MCP 菜单（增删改查，内置多行编辑）
- `/help` - 帮助

**CLI 与后端通信**：
- CLI 启动时通过 `cmd/cli/main.go` 启动 headless HTTP 服务
- CLI 进程与 Go 后端通过 WebSocket 通信
- 支持工具调用审批（内置审批界面）

## Tech Stack

- **后端**：Go 1.26, Chi router, GORM, SQLite, Gorilla WebSocket, Anthropic Go SDK, OpenAI Go SDK, ONNX Runtime, Chroma
- **前端**：Vue 3, Vite 7, TypeScript, Tailwind CSS 4, Pinia, CodeMirror, vue-i18n
- **CLI**：React 19, Ink, TypeScript, tsup, argparse, ws
- **日志**：zap
- **认证**：JWT
