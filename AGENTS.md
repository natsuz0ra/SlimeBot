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
├─ shared/
│  └─ chat-protocol/         # WebSocket contracts, golden fixtures, and history projection
├─ prompts/                  # Embedded prompts and templates
├─ scripts/                  # Release packaging and install scripts
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
├─ acp/                      # Agent Client Protocol (ACP) support
│  ├─ agent/                 # `slimebot acp` agent-side bridge over ChatService
│  └─ client/                # External ACP agent process pool, sessions, and permission bridge
├─ app/                      # App composition/wiring
├─ apperrors/                # Shared application errors
├─ auth/                     # Auth primitives
├─ config/                   # Runtime configuration
├─ constants/                # Shared constants
├─ domain/                   # Domain models, provider-neutral LLM value types, and shared service/tool DTOs
├─ logging/                  # Logger setup and helpers
├─ mcp/                      # MCP protocol integration
├─ platforms/
│  └─ telegram/              # Telegram platform adapter
├─ repositories/             # Persistence/data access, versioned schema migrations, and atomic scheduled-run transactions
├─ runtime/                  # Host state, lifecycle, and managed agent execution
│  ├─ agent/                 # Durable activations, inbox scheduling, root budgets, and database ownership
│  └─ workspace/             # Isolated worktrees, integration journals, and main-session file checkpoints
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
│  ├─ chat/                  # Chat orchestration, tool dispatch, subagent execution, and active-turn steering
│  ├─ context/               # Model transcript projection, per-request budgets, pruning, and atomic checkpoints
│  ├─ config/                # Config service logic
│  ├─ llm/                   # LLM abstraction/service
│  ├─ memory/                # Long-term file memory store/service
│  ├─ openai/                # OpenAI provider integration
│  ├─ plan/                  # Planning workflow logic
│  ├─ privacy/               # Local request masking, response restoration, and model lifecycle
│  │  └─ inference/          # Pure Go Unigram tokenizer and fixed INT8 BERT inference; offline reference fixtures in testdata/
│  ├─ schedule/              # Scheduled chat task CRUD and scheduler
│  ├─ session/               # Session lifecycle/state
│  ├─ settings/              # User/system settings
│  ├─ skill/                 # Skill loading/execution
│  ├─ subagent/              # Persistent agents, scoped messaging, task DAG/CAS, and artifact acceptance
│  ├─ terminal/              # Interactive PTY sessions, replay, and process lifecycle
│  ├─ team/                  # Legacy Agent Team history and compatibility records
│  ├─ usage/                 # Provider-reported token accounting and session output budgets
│  └─ workspace/             # Read-only session file previews and Git status/diffs
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
│  ├─ home/                  # Home page, scheduled task workspace, and run detail components
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
- Shared chat wire contract and history timeline: `internal/server/ws/events.go`, `shared/chat-protocol/` (Vue and CLI decoding, projection, and cross-surface golden fixtures)
- Integrated terminal: `internal/services/terminal/`, `internal/server/controller/terminal.go`, `internal/server/ws/terminal.go`, `frontend/src/components/chat/TerminalDrawer.vue`
- Read-only workspace browser: `internal/services/workspace/`, `internal/server/controller/workspace.go`, `frontend/src/components/chat/WorkspaceDrawer.vue`
- ACP agent mode (`slimebot acp`): `internal/acp/agent/`, `internal/app/acp.go`, stdout reserved for JSON-RPC (logs redirected to stderr in `cmd/server/main.go`)
- External ACP agents: `internal/acp/client/`, `internal/services/chat/acp.go`, `internal/services/config/acp_config.go`, `internal/repositories/repo_acp_configs.go`, `internal/server/controller/acp_configs.go`, `internal/command/command.go`
- Web ACP settings and external engine picker: `frontend/src/components/settings/SettingsACPTab.vue`, `frontend/src/api/acp.ts`, `frontend/src/utils/acp.ts`, `frontend/src/composables/home/useHomeModelSelector.ts`
- Sandbox policy and command isolation: `internal/sandbox/`
- LLM abstraction/provider wiring: `internal/services/llm/` (compatibility aliases), `internal/domain/llm_types.go` (provider-neutral values), `internal/services/openai/`, `internal/services/anthropic/`
- Turn tool execution: `internal/services/chat/tool_dispatcher.go`, `internal/services/chat/special_tool_handlers.go`, `internal/tools/runtime_context.go` (per-turn dependencies, tool batches, approvals, and special handlers)
- Nested subagent execution: `internal/services/chat/agent_run_subagent.go` (shared executor contract and team lifecycle for cooperative and direct execution)
- Active-turn steering: `internal/services/chat/steering.go`, `internal/services/chat/agent.go`, `AGENT-FLOW.md`
- Web conversation history navigation: `frontend/src/components/chat/ChatHistoryNav.vue`, `frontend/src/composables/home/useHomeScroll.ts`
- Conversation pinning and export: `internal/repositories/repo_sessions.go`, `internal/services/session/export.go`, `internal/server/controller/session_export.go`, `frontend/src/components/chat/SessionExportDialog.vue`
- Web slash commands for workbench actions and enabled skills: `frontend/src/components/chat/SlashCommandMenu.vue`, `frontend/src/utils/slashCommands.ts`
- Background command completion, persistence and manual recovery: `internal/tools/process.go`, `internal/tools/exec.go`, `internal/services/chat/background_tasks.go`, `internal/services/chat/background_recovery.go`, `AGENT-FLOW.md`
- Context compaction: `internal/services/context/`, `internal/repositories/repo_context.go`, `internal/services/chat/context_bridge.go` (raw model steps are separate from UI messages; usage reads never call models; `SLIMEBOT_CONTEXT_COMPACTION=off` bypasses checkpoints while keeping pruning/budget guards)
- Sensitive data masking, encrypted mapping retention and local model lifecycle: `internal/services/privacy/`, `internal/server/controller/privacy.go`, `internal/server/controller/session_privacy.go`
- Main-session file checkpoints and rollback: `internal/runtime/workspace/checkpoints.go`, `internal/services/chat/file_checkpoints.go`, `internal/server/controller/file_checkpoints.go`, `frontend/src/components/chat/ChatCheckpointDialog.vue`
- Pure Go privacy inference and tokenizer reference tests: `internal/services/privacy/inference/`
- Long-term memory store/service: `internal/services/memory/`, `internal/tools/memory.go`
- Scheduled tasks and execution history: `internal/services/schedule/`, `internal/tools/schedule.go`, `internal/app/schedule_runner.go`, `internal/server/controller/scheduled_tasks.go` (claim/completion use atomic store transactions)
- Home scheduled task workspace: `frontend/src/components/home/ScheduledTasksWorkspace.vue`, `frontend/src/components/home/TaskRunTimeline.vue`, `frontend/src/api/schedule.ts` (sidebar entry and `/tasks` route; execution contexts are excluded from ordinary chats)
- Unified task center: `internal/app/task_center.go`, `internal/server/controller/task_center.go`, `frontend/src/api/taskCenter.ts` (background commands, subagents and scheduled runs in the `/tasks` overview)
- Token usage summaries and output budgets: `internal/services/usage/`, `internal/services/llm/request_observer.go`, `internal/repositories/repo_usage.go`, `frontend/src/components/settings/SettingsUsageTab.vue`
- Persistent subagents and collaboration: `internal/services/subagent/`, `internal/runtime/agent/`, `internal/runtime/workspace/`, `internal/services/chat/cooperative.go`, `internal/server/controller/cooperative.go` (hidden sessions, durable inbox, budgets, isolated worker artifacts, validation and recoverable integration)
- Web collaboration workspace: `frontend/src/components/chat/CooperativePanel.vue`, `frontend/src/api/cooperative.ts` (agents, shared task board, artifacts and pending approvals; available in chat and scheduled run details)
- CLI collaboration: `cli/src/components/CooperativeView.tsx`, `/agents` routing in `cli/src/controllers/commands.ts` and `cli/src/app.tsx`
- Legacy Agent Team compatibility: `internal/services/team/`, `internal/services/chat/team_runtime.go`
- Tool implementations: `internal/tools/`
- Update checking/apply flow: `internal/updater/`, `internal/server/controller/update.go`
- Build/version metadata: `internal/version/`
- Web settings page UI: `frontend/src/components/settings/`, `frontend/src/composables/settings/`, `frontend/src/pages/`
- Web memory settings: `frontend/src/components/settings/SettingsMemoryTab.vue`, `frontend/src/api/memory.ts`
- Web privacy settings and previews: `frontend/src/components/settings/SettingsPrivacyTab.vue`, `frontend/src/api/privacy.ts`
- Web update center: `frontend/src/components/settings/SettingsAboutTab.vue`, `frontend/src/api/update.ts`
- CLI interaction flow: `cmd/cli/`, `cli/src/controllers/`, `cli/src/components/`, `cli/src/ws/`
- CLI memory command: `/memory` routing in `cli/src/controllers/commands.ts`, API in `cli/src/api/client.ts`
- CLI background task controls: `/background` routing in `cli/src/controllers/commands.ts`, events in `cli/src/hooks/useCliSocket.ts`
- CLI privacy commands: `/privacy` routing in `cli/src/controllers/commands.ts`, parsing and formatting in `cli/src/utils/privacy.ts`
- CLI update view: `cli/src/components/UpdateView.tsx`, `/update` command routing in `cli/src/controllers/commands.ts`
- Prompt templates: `prompts/`
- Release/install packaging: `scripts/`
- GitHub Release automation: `.github/workflows/release.yml`
- Desktop host entry: `internal/app/desktop.go`, `cmd/server/main.go`
- Desktop packaging and preview CI: `desktop/`, `.github/workflows/desktop-preview.yml`
- Bundled ripgrep binaries for release packages: `third_party/ripgrep/`

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
- Optional real privacy-model integration: set `SLIMEBOT_PRIVACY_TEST_MODEL_DIR` to the verified AstrLink Guard model directory, then run `go test ./internal/services/privacy/...`. This covers pure Go inference, publisher tokenizer/ONNX reference fixtures and model lifecycle without Python. Normal tests do not download models.

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
