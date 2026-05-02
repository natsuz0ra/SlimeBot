<p align="center">
  <img src="assets/title.png" alt="SlimeBot Logo" width="420" />
  <br /><br />
  <strong>English</strong> | <a href="README.zh-CN.md">简体中文</a>
</p>

# SlimeBot

A personal AI agent demo: an extensible foundation for conversational AI apps. It ships with a **Go** backend, a **Vue 3** web UI, and a **React + Ink** terminal CLI.

## Features

- **Chat & sessions**
  - Session list, create, rename, delete
  - Per-session message history
  - Real-time streaming replies
  - Auto-generated session titles with live updates
  - Multimodal support
- **Tools & agent**
  - Multi-turn agent tool-call flow
  - Approval modes: **standard** (manual confirm for sensitive tools) and **auto** (execute directly)
  - User approval for sensitive built-in tools (today: `exec`) in web UI, CLI, and Telegram flows
  - Tool results stored in history with detail views
  - Built-in tools: `command line`, `web request`, `web search` (Tavily), `to-do`
  - Supports coding-agent-like file read/write capabilities for text editing workflows
  - **Subagent:** the main agent can delegate a self-contained task to an inner agent with **isolated context** (no parent chat history). Only **one nesting level** is allowed (the subagent cannot call `run_subagent` again). Inner tool calls are shown **nested under** the parent tool in the web UI and CLI; session history stores `parentToolCallId` so grouping survives a reload.
- **Planning & reasoning controls**
  - Plan mode for “draft first, execute after approval” workflow
  - Plan lifecycle: generate, approve/reject, modify-and-regenerate, execute
  - Thinking level controls (`off` / `low` / `medium` / `high`) for model reasoning depth
  - Thinking stream events and timeline rendering in both web UI and CLI
- **Context compression**
  - Rolling session summaries
  - Long-context compression with recent-message backfill
- **Configuration & extensions**
  - MCP configuration management
  - Skills: upload, list, delete, runtime activation
- **Messaging platforms** (Telegram today)
  - Platform configuration management
  - Inbound messages and replies
- **CLI TUI**
  - Standalone CLI for chat and basic configuration (headless Go child + Ink UI)

## Screenshots

### Sign-in

![Sign-in](assets/login.png)

### Home

![Home](assets/home.png)

### Chat

![Chat](assets/chat.png)

### Tool execution

![Tool execution](assets/tool_exec.png)

### Telegram

<img src="assets/tg_chat.jpg" alt="Telegram preview" width="220" />

### CLI

<img src="assets/cli.png" alt="CLI" width="800" />

## Architecture & stack

- **Production**: one Go binary serves REST/WebSocket and embeds the web UI from `web/dist` (`go:embed`).
- **Development**: `npm run dev` runs the Go server and Vite; Vite proxies `/api` and `/ws` to port `8080`.
- **Data**: SQLite by default at `~/.slimebot/storage/data.db`.
- **Context compression**: hidden per-session summaries are stored in SQLite and used only for model context.

**Stack (high level):** Go backend · Vue 3 web app · React + Ink CLI.

## Getting started

Default ports: backend **8080**, Vite **5173**.

From the repo root:

```bash
make deps
npm run dev
```

Or install manually:

```bash
npm install
npm install --prefix frontend
npm run dev
```

On first run, `~/.slimebot/.env` is created if missing; missing keys from the embedded template are appended over time.

**First-time login (web server):** if no user exists yet, a default account is seeded (**username `admin`**, password **`admin`**) and you are prompted to change the password. Change it immediately for anything beyond local development.

**Production build** (outputs `slimebot` with the embedded frontend):

```bash
npm run build
# or
make build
```

**Run the server only** (API + static UI after a build):

```bash
go run ./cmd/server/main.go
```

**CLI TUI:**

```bash
npm run cli
```

`make cli` installs CLI dependencies, builds the Ink bundle, and produces a `slimebot-cli` binary (see [Makefile](Makefile)).

**Tests:**

```bash
make test
# or
go test ./...
```

**Docker:**

```bash
make docker-build
make docker-run
```

**Docker Compose:**

```bash
make compose-up
make compose-down
```

### CLI commands

- `/new` — new session (lazy: created on first message)
- `/session` — switch / delete sessions
- `/model` — set default model
- `/skills` — view / remove skills
- `/mcp` — MCP CRUD with multiline editor
- `/mode` — toggle approval mode (`standard` / `auto`)
- `/effort` — set thinking level (`off` / `low` / `medium` / `high`)
- `/plan` — toggle plan mode (`on` / `off`)
- `/help` — help

## Data layout (`~/.slimebot`)

```text
~/.slimebot/
  .env
  skills/
  storage/
    data.db
    chat_uploads/
```

- `.env` — configuration
- `storage/data.db` — SQLite
- `storage/chat_uploads` — chat attachments
- `skills/` — installed skills

## Configuration (`~/.slimebot/.env`)

Variables read by the server (defaults shown where applicable):

- `SERVER_PORT` — HTTP port (default `8080`)
- `DB_PATH` — SQLite path (default `~/.slimebot/storage/data.db`)
- `SKILLS_ROOT` — skills root (default `~/.slimebot/skills`)
- `CHAT_UPLOAD_ROOT` — uploads (default `~/.slimebot/storage/chat_uploads`)
- `FRONTEND_ORIGIN` — set to `http://localhost:5173` when using Vite; empty for same-origin production
- `WEB_SEARCH_API_KEY` — Tavily API key for `web_search`
- `JWT_SECRET` — **required in server mode**; server fails to start if unset (CLI headless mode can auto-generate one)
- `JWT_EXPIRE` — JWT lifetime in minutes (default `21600` ≈ 15 days)
- `approvalMode` (app setting) — `standard` or `auto`
- `thinkingLevel` (app setting) — `off` / `low` / `medium` / `high`

The file created on first boot follows the embedded template in [internal/runtime/env.template](internal/runtime/env.template). You can add the optional keys above manually if needed.

Example:

```env
SERVER_PORT=8080
DB_PATH=~/.slimebot/storage/data.db
SKILLS_ROOT=~/.slimebot/skills
CHAT_UPLOAD_ROOT=~/.slimebot/storage/chat_uploads
WEB_SEARCH_API_KEY=YOUR_TAVILY_API_KEY
JWT_SECRET=CHANGE_ME_TO_A_RANDOM_SECRET
JWT_EXPIRE=21600

# FRONTEND_ORIGIN=http://localhost:5173
```

### Frontend (`frontend/.env`)

- `VITE_API_BASE_URL` — HTTP base (e.g. `http://localhost:8080`)
- `VITE_WS_URL` — WebSocket base (e.g. `ws://localhost:8080`)

Example:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

## Status & roadmap

**Done**

- Sessions and WebSocket streaming (including errors, tool-call, subagent, and thinking events)
- Agent tools and approvals (`exec` requires confirmation in standard mode; optional auto approval mode)
- Plan mode with plan generation, approve/reject/modify flow, and execution after approval
- Thinking level controls (`off` / `low` / `medium` / `high`) with streamed reasoning display
- Subagent / nested agent (`run_subagent`), nested tool UI, and persisted parent linkage in tool-call history
- MCP and skills
- Context compression with rolling hidden session summaries
- Telegram integration
- Multimodal chat
- JWT auth and default admin bootstrap

**Planned**

- More messaging platforms (e.g. Discord, Slack)

## License

This project is licensed under the [MIT License](LICENSE).
