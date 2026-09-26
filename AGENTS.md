# Repository Guidelines

## Project Structure & Module Organization

SlimeBot is a single repository with four main surfaces: Go backend, Vue web frontend, React + Ink CLI, and Electron desktop.

Top-level navigation:

```text
.
├─ .github/
│  └─ workflows/             # GitHub Actions CI/release workflows
├─ cmd/                      # Go entrypoints
│  ├─ server/                # HTTP/WebSocket server bootstrap
│  └─ cli/                   # Headless CLI bridge bootstrap
├─ internal/                 # Backend application code
├─ frontend/                 # Vue 3 web app
├─ cli/                      # React + Ink terminal app
├─ desktop/                  # Electron window, tray, updater, and installers
├─ prompts/                  # Embedded prompts and templates
├─ scripts/                  # Release packaging and install scripts
├─ docs/                     # Manual deployment and longer-form docs
├─ third_party/              # Third-party runtime binaries copied into release packages
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
├─ config/                   # Runtime configuration
├─ constants/                # Shared constants
├─ domain/                   # Domain models/types
├─ logging/                  # Logger setup and helpers
├─ mcp/                      # MCP protocol integration
├─ platforms/
│  └─ telegram/              # Telegram platform adapter
├─ repositories/             # Persistence/data access
├─ runtime/                  # Runtime state and lifecycle
├─ sandbox/                  # Tool sandbox policies and OS sandbox runners
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
│  ├─ memory/                # Long-term file memory store/service
│  ├─ openai/                # OpenAI provider integration
│  ├─ plan/                  # Planning workflow logic
│  ├─ schedule/              # Scheduled chat task CRUD and scheduler
│  ├─ session/               # Session lifecycle/state
│  ├─ settings/              # User/system settings
│  └─ skill/                 # Skill loading/execution
├─ tools/                    # Tool implementations
├─ updater/                  # Release update checking/apply helper
└─ version/                  # Build/version metadata
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

Desktop structure (`desktop/`):

```text
desktop/
├─ main.cjs                   # Electron process, tray, Go host, and updater
├─ preload.cjs                # Narrow IPC bridge for desktop updates
├─ electron-builder.config.cjs # macOS/Windows installer configuration
├─ resources/                 # App and tray icons derived from the project logo
└─ scripts/                   # Backend, ripgrep, and icon packaging
```

Quick index (feature -> first place to inspect):

- HTTP route definitions: `internal/server/router/`
- HTTP controller behavior: `internal/server/controller/`
- WebSocket server flow: `internal/server/ws/`
- Sandbox policy and command isolation: `internal/sandbox/`
- LLM abstraction/provider wiring: `internal/services/llm/`, `internal/services/openai/`, `internal/services/anthropic/`
- Long-term memory store/service: `internal/services/memory/`, `internal/tools/memory.go`
- Scheduled chat tasks: `internal/services/schedule/`, `internal/tools/schedule.go`
- Tool implementations: `internal/tools/`
- Update checking/apply flow: `internal/updater/`, `internal/server/controller/update.go`
- Build/version metadata: `internal/version/`
- Web settings page UI: `frontend/src/components/settings/`, `frontend/src/composables/settings/`, `frontend/src/pages/`
- Web memory settings: `frontend/src/components/settings/SettingsMemoryTab.vue`, `frontend/src/api/memory.ts`
- Web update center: `frontend/src/components/settings/SettingsAboutTab.vue`, `frontend/src/api/update.ts`
- CLI interaction flow: `cmd/cli/`, `cli/src/controllers/`, `cli/src/components/`, `cli/src/ws/`
- CLI memory command: `/memory` routing in `cli/src/controllers/commands.ts`, API in `cli/src/api/client.ts`
- CLI update view: `cli/src/components/UpdateView.tsx`, `/update` command routing in `cli/src/controllers/commands.ts`
- Prompt templates: `prompts/`
- Release/install packaging: `scripts/`
- GitHub Release automation: `.github/workflows/release.yml`
- Desktop host entry: `internal/app/desktop.go`, `cmd/server/main.go`
- Desktop packaging and preview CI: `desktop/`, `.github/workflows/desktop-preview.yml`
- Bundled ripgrep binaries for release packages: `third_party/ripgrep/`
- Manual deployment docs: `docs/`

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
- `make package`: build cross-platform Release archives under `dist/`.
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

Recent history uses concise messages like `update: 更新版本号`, `update: 支持一条命令安装`, and `update: 前端ai消息中的代码块新增复制按钮`; follow `type: short summary`, usually `update:` for incremental changes. Keep summaries short, imperative or noun-phrase style, and prefer Chinese descriptions consistent with the existing history. Merge commits may keep the hosting platform's default format, for example `Merge branch 'dev/1.26.0'` or `Merge pull request #29 from natsuz0ra/dev/1.25.0`.

Use release branch names in the `dev/x.y.z` format, matching existing branches such as `dev/1.24.0`, `dev/1.25.0`, and `dev/1.26.0`. For patch releases, continue the same pattern, for example `dev/1.26.1`. Use other prefixes only when the branch is not a release/version branch and the purpose is clear.

Pull requests should include a clear summary, test commands run, linked issues when available, and screenshots or short recordings for visible Web/CLI UI changes.

## Release Publishing Notes

Official Release packages are built on GitHub Actions, not locally. After a release branch such as `dev/1.27.1` is merged into `main`, create and push the matching stable tag from the merged `main` commit:

```bash
git checkout main
git pull origin main
git tag v1.27.1
git push origin v1.27.1
```

The tag push triggers `.github/workflows/release.yml`, which runs `scripts/package-release.sh` on GitHub and publishes the Release assets. Local packaging commands may be used for verification, but they are not the official publishing path.

Release PR descriptions should use the existing Chinese numbered-list style and include the verification commands that were actually run:

```markdown
1. 更新版本号或修复摘要
2. 关键行为变更
3. 测试或文档补充

验证：
- npm --prefix cli test
- go test ./...
```

GitHub Release notes should use the current bilingual format:

```markdown
## English

- Short user-facing release note.

## 中文

- 简短的面向用户的发布说明。
```

## Security & Configuration Tips

Runtime data defaults to `~/.slimebot`. Do not commit `config.cfg`, legacy `.env`, SQLite data, uploads, API keys, JWT secrets, or local `.slimebot` directories. Server mode requires `JWT_SECRET`; CLI headless mode can generate one automatically.
