# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SlimeBot 是一个个人 AI Agent Demo 项目，实现可扩展的 AI 会话应用雏形。后端为 Go，前端为 Vue 3，CLI 为 React+Ink (Node.js)。生产环境通过 `go:embed` 嵌入前端静态资源构建单一二进制文件。

## Build & Run Commands

```bash
# 安装依赖（根目录 + frontend）
make deps

# 开发模式（concurrently 启动 Go 后端 + Vite 前端，Vite 代理 /api /ws 到 8080）
npm run dev

# 生产构建（前端先构建到 web/dist/，Go 再 embed 生成 slimebot 二进制）
make build

# CLI 构建
make cli

# 运行 CLI（开发模式，自动构建 + 启动）
npm run cli

# 运行测试
make test

# 运行单个包的测试
go test ./internal/services/chat/...

# 带 race 检测的测试
go test -race ./...

# 测试覆盖率
go test -cover ./...

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

## Configuration

运行时配置通过 `~/.slimebot/.env` 管理，首次启动自动生成。关键必填项：`JWT_SECRET`。默认端口：后端 `8080`，Vite `5173`。前端环境变量在 `frontend/.env`（`VITE_API_BASE_URL`、`VITE_WS_URL`）。

## Code Conventions

- 不可变模式：始终创建新对象，不修改已有对象
- 错误处理：使用 `fmt.Errorf("context: %w", err)` 包装错误上下文
- 文件组织：小文件优先（200-400 行典型，800 行上限），按功能/领域组织
- Go 接口：保持小接口（1-3 个方法），接受接口返回结构体
- 表驱动测试
- Commit 格式：`<type>: <description>`（feat, fix, refactor, docs, test, chore, perf, ci）

## Tech Stack

- **后端**：Go 1.26, Chi router, GORM, SQLite, Gorilla WebSocket, OpenAI Go SDK, ONNX Runtime, Chroma
- **前端**：Vue 3, Vite 7, TypeScript, Tailwind CSS 4, Pinia, CodeMirror
- **CLI**：React 19, Ink, TypeScript, tsup, argparse
- **日志**：zap
- **认证**：JWT
