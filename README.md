<p align="center">
  <img src="frontend/public/slime-icon.svg" alt="SlimeBot Logo" width="96" />
</p>

<h1 align="center">SlimeBot</h1>

这是个人练手用的 Agent Demo 项目，目标是实现一个可持续扩展的 AI 会话应用雏形。

## 1. 当前支持功能

- 基本会话功能：会话记录以及上下文能力
- 消息能力：基于 WebSocket 的流式回复
- tools能力：支持调用工具为模型提供外部感知能力

## 2. 架构与技术栈

### 架构说明

项目采用前后端分离架构：

- 前端通过 HTTP 调用后端 REST API（会话与设置相关）
- 前端通过 WebSocket 与后端进行实时聊天流式通信
- 后端通过服务层访问模型接口，并将数据持久化到 SQLite

### 技术栈

- 前端：Vue 3
- 后端：Go

## 3. 如何启动

> 默认端口：后端 `8080`，前端 `5173`

### Windows（PowerShell）

```powershell
# 1) 启动后端
cd G:\gitCode\SlimeBot\backend
go mod tidy
go run .\cmd\server\main.go
```

```powershell
# 2) 启动前端（新开一个终端）
cd G:\gitCode\SlimeBot\frontend
npm install
Copy-Item .env.example .env
npm run dev
```

### macOS（zsh/bash）

```bash
# 1) 启动后端
cd /path/to/SlimeBot/backend
go mod tidy
go run ./cmd/server/main.go
```

```bash
# 2) 启动前端（新开一个终端）
cd /path/to/SlimeBot/frontend
npm install
cp .env.example .env
npm run dev
```

### Linux（bash）

```bash
# 1) 启动后端
cd /path/to/SlimeBot/backend
go mod tidy
go run ./cmd/server/main.go
```

```bash
# 2) 启动前端（新开一个终端）
cd /path/to/SlimeBot/frontend
npm install
cp .env.example .env
npm run dev
```

## 4. 配置文件使用方法

### 后端配置：`backend/.env`

后端启动时会读取环境变量：

- `SERVER_PORT`：服务端口，默认 `8080`
- `DB_PATH`：SQLite 文件路径，默认 `./storage/data.db`
- `FRONTEND_ORIGIN`：允许跨域的前端地址，默认 `http://localhost:5173`
- `WEB_SEARCH_API_KEY`：tavily网络搜索api_key

示例：

```env
SERVER_PORT=8080
DB_PATH=./storage/data.db
FRONTEND_ORIGIN=http://localhost:5173
```

### 前端配置：`frontend/.env`

- `VITE_API_BASE_URL`：后端 HTTP 地址（例如 `http://localhost:8080`）
- `VITE_WS_URL`：后端 WebSocket 地址（例如 `ws://localhost:8080`）

示例：

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

## 5. 功能状态与待办

### 已完成

- 基本的会话功能
- tools
  - exec: 系统命令行执行器
  - http_request: http请求器
  - web_search: 基于tavily的网络搜索器
- mcp
- skills

### 待完成功能

- 前端重构美化
