<p align="center">
  <img src="assets/title.png" alt="SlimeBot Logo" width="420" />
</p>

这是个人练手用的 Agent Demo 项目，目标是实现一个可持续扩展的 AI 会话应用雏形。

## 1. 当前支持功能

- 会话与消息
  - 会话列表、创建、重命名、删除
  - 按会话拉取历史消息
  - 基于 WebSocket 的实时流式回复（start/chunk/done）
  - 会话标题自动生成与更新推送
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

## 2. UI 预览

### 登录页

![登录页预览](assets/login.png)

### 主页

![主页预览](assets/home.png)

### 会话页

![会话页预览](assets/chat.png)

## 3. 架构与技术栈

### 架构说明

项目采用前后端分离架构：

- 前端通过 HTTP 调用后端 REST API（会话与设置相关）
- 前端通过 WebSocket 与后端进行实时聊天流式通信
- 后端通过服务层访问模型接口，并将数据持久化到 SQLite

### 技术栈

- 前端：Vue 3
- 后端：Go

## 4. 如何启动

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

## 5. 配置文件使用方法

### 后端配置：`backend/.env`

后端启动时会读取环境变量：

- `SERVER_PORT`：服务端口，默认 `8080`
- `DB_PATH`：SQLite 文件路径，默认 `./storage/data.db`
- `FRONTEND_ORIGIN`：允许跨域的前端地址，默认 `http://localhost:5173`
- `WEB_SEARCH_API_KEY`：tavily网络搜索api_key
- `JWT_SECRET`：JWT 签名密钥（必填，未配置将启动失败）
- `JWT_EXPIRE`：JWT 过期时间（单位：分钟，默认 `21600` 即 15 天）

示例：

```env
SERVER_PORT=8080
DB_PATH=./storage/data.db
FRONTEND_ORIGIN=http://localhost:5173
JWT_SECRET=CHANGE_ME_TO_A_RANDOM_SECRET
JWT_EXPIRE=21600
```

### 前端配置：`frontend/.env`

- `VITE_API_BASE_URL`：后端 HTTP 地址（例如 `http://localhost:8080`）
- `VITE_WS_URL`：后端 WebSocket 地址（例如 `ws://localhost:8080`）

示例：

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

## 6. 功能状态与待办

### 已完成

- 会话管理与消息流式回复（WebSocket）
- Agent 工具调用与审批流程
  - `exec`：系统命令行执行器
  - `http_request`：HTTP 请求器
  - `web_search`：基于 Tavily 的网络搜索器
- MCP 配置与工具执行能力
- Skills 包管理与运行时激活
- 会话持久化记忆与主动检索

### 待完成功能

- 消息平台接入能力
