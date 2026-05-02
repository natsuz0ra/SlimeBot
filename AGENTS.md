# Repository Guidelines

## Project Structure & Module Organization

SlimeBot is a single repository with three main surfaces: Go backend, Vue web frontend, and React + Ink CLI.

Top-level navigation:

```text
.
├─ cmd/                      # Go entrypoints
│  ├─ server/                # HTTP/WebSocket server bootstrap
│  └─ cli/                   # Headless CLI bridge bootstrap
├─ internal/                 # Backend application code
├─ frontend/                 # Vue 3 web app
├─ cli/                      # React + Ink terminal app
├─ prompts/                  # Embedded prompts and templates
├─ web/                      # Built frontend assets served by backend
└─ assets/                   # README screenshots and images
```

Backend structure (`cmd/` + `internal/`):

```text
cmd/
├─ server/
└─ cli/

internal/
├─ app/                      # App composition/wiring
├─ apperrors/                # Shared application errors
├─ auth/                     # Auth primitives
├─ cli/                      # CLI-side backend integration
├─ config/                   # Runtime configuration
├─ constants/                # Shared constants
├─ domain/                   # Domain models/types
├─ logging/                  # Logger setup and helpers
├─ mcp/                      # MCP protocol integration
├─ platforms/
│  └─ telegram/              # Telegram platform adapter
├─ repositories/             # Persistence/data access
├─ runtime/                  # Runtime state and lifecycle
├─ server/
│  ├─ apierrors/             # HTTP API error mapping
│  ├─ controller/            # HTTP handlers/controllers
│  ├─ middleware/            # HTTP middlewares
│  ├─ router/                # Route registration
│  └─ ws/                    # WebSocket handlers
├─ services/
│  ├─ anthropic/             # Anthropic provider integration
│  ├─ auth/                  # Auth service logic
│  ├─ chat/                  # Chat orchestration
│  ├─ config/                # Config service logic
│  ├─ llm/                   # LLM abstraction/service
│  ├─ openai/                # OpenAI provider integration
│  ├─ plan/                  # Planning workflow logic
│  ├─ session/               # Session lifecycle/state
│  ├─ settings/              # User/system settings
│  └─ skill/                 # Skill loading/execution
└─ tools/                    # Tool implementations
```

Frontend structure (`frontend/src`):

```text
frontend/src/
├─ api/                      # HTTP client calls
├─ components/
│  ├─ chat/                  # Chat-specific UI components
│  ├─ home/                  # Home page components
│  ├─ login/                 # Login/auth components
│  ├─ settings/              # Settings UI components
│  └─ ui/                    # Shared base UI components
├─ composables/
│  ├─ chat/                  # Chat composables
│  ├─ home/                  # Home composables
│  └─ settings/              # Settings composables
├─ pages/                    # Route pages/views
├─ stores/                   # Pinia stores
├─ styles/                   # Global styles/themes
├─ types/                    # Frontend shared types
└─ utils/                    # Frontend utilities
```

CLI structure (`cli/src`):

```text
cli/src/
├─ api/                      # Backend/API clients
├─ components/               # Ink UI components
├─ controllers/              # CLI flow/control logic
├─ hooks/                    # Ink/React hooks
├─ native/                   # Native bridge wrappers
├─ types/                    # CLI types/interfaces
├─ utils/                    # CLI utilities
└─ ws/                       # WebSocket client logic
```

Quick index (feature -> first place to inspect):

- HTTP route definitions: `internal/server/router/`
- HTTP controller behavior: `internal/server/controller/`
- WebSocket server flow: `internal/server/ws/`
- LLM abstraction/provider wiring: `internal/services/llm/`, `internal/services/openai/`, `internal/services/anthropic/`
- Tool implementations: `internal/tools/`
- Web settings page UI: `frontend/src/components/settings/`, `frontend/src/composables/settings/`, `frontend/src/pages/`
- CLI interaction flow: `cli/src/controllers/`, `cli/src/components/`, `cli/src/ws/`
- Prompt templates: `prompts/`

Notes:

- Ignore noise directories (for example: `.git/`, `node_modules/`, build outputs) when navigating.
- `web/dist/` is generated output; do not treat it as source of truth for frontend logic.

## Documentation Maintenance

Any repository structure changes must update this file in the same PR.

- If a PR adds, removes, or renames directories relevant to development, update the structure trees and quick index in `AGENTS.md`.
- If module responsibilities move across directories (even without directory renaming), update the corresponding responsibility descriptions.
- PR description must include one of:
  - `Docs sync: AGENTS.md updated`
  - `Docs sync: N/A (<reason>)`

## Build, Test, and Development Commands

- `make deps`: install root and frontend npm dependencies.
- `npm run dev`: run the Go server and Vite dev server together.
- `npm run build` or `make build`: build the frontend into `web/dist` and compile the `slimebot` server binary.
- `npm run cli`: build the Ink CLI and run the Go CLI entrypoint.
- `make test` or `go test ./...`: run backend tests.
- `npm --prefix frontend test`: run frontend Node tests.
- `npm --prefix cli test`: run CLI Node tests.

## Coding Style & Naming Conventions

Use `gofmt`/`go test` conventions for Go. Keep package names short and lowercase, and place tests beside packages as `*_test.go`. TypeScript uses ES modules; prefer explicit, descriptive component and composable names such as `SettingsLLMTab.vue` and `useHomeScroll.ts`. Keep frontend path aliases under `@/` for `frontend/src`. Preserve existing two-space indentation in Vue/TS files and tab indentation in Go.

## Testing Guidelines

Add focused tests near changed behavior. Backend tests use Go's standard `testing` package. Frontend and CLI tests use Node's built-in test runner with `tsx` loaders. Name TypeScript tests `*.test.ts` and Go tests `*_test.go`. For UI logic, prefer reducer, formatter, store, and utility tests over brittle visual assertions.

Directory-aware test targeting hints:

- Backend changes under `internal/` or `cmd/`: run `go test ./...`
- Frontend changes under `frontend/src`: run `npm --prefix frontend test`
- CLI changes under `cli/src`: run `npm --prefix cli test`

## Commit & Pull Request Guidelines

Recent history uses concise messages like `update: cli ui优化` and `update: 支持工具的并行执行与批量审批`; follow `type: short summary`, usually `update:` for incremental changes. Pull requests should include a clear summary, test commands run, linked issues when available, and screenshots or short recordings for visible Web/CLI UI changes.

## Security & Configuration Tips

Runtime data defaults to `~/.slimebot`. Do not commit `.env`, SQLite data, uploads, API keys, JWT secrets, or local `.slimebot` directories. Server mode requires `JWT_SECRET`; CLI headless mode can generate one automatically.
