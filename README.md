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
  - WebSocket streaming (`start` / `chunk` / `done`, plus `error`, tool-call, and subagent events)
  - Auto-generated session titles with live updates
  - Multimodal support
- **Tools & agent**
  - Multi-turn agent tool-call flow
  - Approval modes: **standard** (manual confirm for sensitive tools) and **auto** (execute directly)
  - User approval for sensitive built-in tools (today: `exec`) in web UI, CLI, and Telegram flows
  - Tool results stored in history with detail views
  - Built-in tools: `exec`, `http_request`, `web_search` (Tavily), **`run_subagent`** (nested agent)
  - **Subagent:** the main agent can delegate a self-contained task to an inner agent with **isolated context** (no parent chat history). Only **one nesting level** is allowed (the subagent cannot call `run_subagent` again). Inner tool calls are shown **nested under** the parent tool in the web UI and CLI; session history stores `parentToolCallId` so grouping survives a reload.
  - WebSocket subagent stream: `subagent_start`, `subagent_chunk`, `subagent_done` (in addition to `tool_call_start` / `tool_call_result`)
- **Planning & reasoning controls**
  - Plan mode for “draft first, execute after approval” workflow
  - Plan lifecycle: generate, approve/reject, modify-and-regenerate, execute
  - Thinking level controls (`off` / `low` / `medium` / `high`) for model reasoning depth
  - Thinking stream events and timeline rendering in both web UI and CLI
- **Memory**
  - Rolling session summaries
  - Long-context compression with recent-message backfill
  - Cross-session retrieval from file-backed markdown memories, indexed for full-text search
- **Configuration & extensions**
  - LLM profiles (add / remove / list)
  - MCP servers (CRUD, enable/disable) and tool loading
  - Skills: upload, list, delete, runtime activation
- **Messaging platforms** (Telegram today)
  - Platform config (create, update, enable/disable)
  - Inbound messages and replies
- **CLI TUI**
  - Standalone CLI for chat and basic configuration (headless Go child + Ink UI)
- **Web UI**
  - Optional internationalization (e.g. English / Chinese) via vue-i18n

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
- **Memory**: markdown files under `~/.slimebot/memory` with a local full-text index for search and prompt injection.

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
  memory/
    MEMORY.md
    index.bleve/
    *.md
  storage/
    data.db
    chat_uploads/
```

- `.env` — configuration
- `memory/` — markdown memory entries, manifest (`MEMORY.md`), and full-text index data under `index.bleve/`
- `storage/data.db` — SQLite
- `storage/chat_uploads` — chat attachments
- `skills/` — installed skills

## Memory storage (how it works)

- Each memory is a Markdown file with YAML frontmatter under `MEMORY_DIR` (default `~/.slimebot/memory`).
- A Bleve full-text index under `memory/index.bleve/` powers search and cross-session recall.
- On startup the server rebuilds the index from disk (see `Core.WarmupInBackground`).

## Configuration (`~/.slimebot/.env`)

Variables read by the server (defaults shown where applicable):

- `SERVER_PORT` — HTTP port (default `8080`)
- `DB_PATH` — SQLite path (default `~/.slimebot/storage/data.db`)
- `SKILLS_ROOT` — skills root (default `~/.slimebot/skills`)
- `CHAT_UPLOAD_ROOT` — uploads (default `~/.slimebot/storage/chat_uploads`)
- `MEMORY_DIR` — memory markdown + index root (default `~/.slimebot/memory`)
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
MEMORY_DIR=~/.slimebot/memory
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
- File-backed persistent memory with full-text search and prompt injection
- Telegram integration
- Multimodal chat
- JWT auth and default admin bootstrap

**Planned**

- More messaging platforms (e.g. Discord, Slack)

## License

This project is licensed under the [MIT License](LICENSE).
