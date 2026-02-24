# Meshtastic Firmware Builder - TODO Plan

## 0) Product Questions (need user confirmation)
- [x] Q1. Scope: any git URL.
- [x] Q2. Ref selection: user chooses branch/tag/commit.
- [x] Q3. Build isolation: run builds in Docker container.
- [x] Q4. Concurrency: default 1, configurable.
- [x] Q5. Retention: one week.
- [x] Q6. Auth: no authentication for MVP.
- [x] Q7. Device discovery: from `variants/` directory.
- [x] Q8. UI language: RU/EN.
- [x] Q9. Deployment: local, prepare Dockerfile(s).

## 1) Architecture Decision
- [x] Backend language: Go (recommended).
  - Why: robust subprocess control, simple concurrent job orchestration, easy SSE streaming, single static binary deployment.
- [x] Frontend: Node.js + React + Vite + TypeScript.
  - Why: fast UI iteration, simple SSE client support, straightforward build-log terminal-like UI.
- [x] API style: REST for commands/state + SSE for real-time logs.
- [x] Storage strategy (MVP): filesystem-based job/artifact storage (no DB initially).

## 2) Project Bootstrap
- [x] Create repository layout:
  - `backend/` (Go API + worker + build runner)
  - `frontend/` (React UI)
  - `build-workdir/` (temporary clones/build outputs, gitignored)
- [x] Add root `.gitignore`, `README.md`, and env sample file.
- [x] Add local run scripts (`make` and npm/go scripts).

## 3) Backend Core (Go)
- [x] Implement config loader (ports, workspace path, timeouts, concurrency, retention).
- [x] Implement structured logging and request IDs.
- [x] Implement HTTP server and API routes:
  - `POST /api/repos/discover`
  - `POST /api/jobs`
  - `GET /api/jobs/{jobId}`
  - `GET /api/jobs/{jobId}/logs` (snapshot)
  - `GET /api/jobs/{jobId}/logs/stream` (SSE)
  - `GET /api/jobs/{jobId}/artifacts`
  - `GET /api/jobs/{jobId}/artifacts/{artifactId}`

## 4) Repository Discovery
- [x] Validate and sanitize repo URL at API boundary.
- [x] Clone/fetch repository into isolated job workspace.
- [x] Read devices from `variants/` directory.
- [x] Return device list and metadata to frontend.

## 5) Build Orchestration
- [x] Implement in-memory job manager with statuses: `queued`, `running`, `success`, `failed`, `cancelled`.
- [x] Implement worker pool with concurrency limit.
- [x] Implement secure command execution:
  - `pio run -e <env>` with strict env-name validation
  - hard timeout + max log size guard
- [x] Stream stdout/stderr into job log buffer and SSE broadcaster.
- [x] Collect firmware artifacts from `.pio/build/<env>/` and register downloadable files.

## 6) Frontend (Node.js)
- [x] Page 1 flow: repo URL input + "Discover devices" action.
- [x] Render discovered environments as selectable device list.
- [x] Start build action and show build status timeline.
- [x] Live log viewer (auto-scroll, error handling).
- [x] Artifacts panel with download links when build succeeds.
- [x] Friendly error states (invalid URL, clone failure, missing env, build failure).

## 7) Security and Reliability
- [x] Restrict allowed protocols and sanitize filesystem paths.
- [x] Prevent command injection by never using shell interpolation.
- [x] Enforce per-job workspace isolation and cleanup policy.
- [x] Add basic rate limiting for build creation endpoint.
- [x] Add CORS config for local UI domain.

## 8) Testing
- [x] Backend unit tests:
  - URL validation
  - variants parser
  - artifact collector
  - job state transitions
- [ ] Backend integration tests (mock runner):
  - discover -> select env -> build -> artifact listing
  - failure and timeout scenarios
- [ ] Frontend tests:
  - core user flow rendering
  - SSE log stream handling

## 9) Documentation
- [x] Update `README.md` with setup, run, and user flow.
- [x] Document API endpoints and example responses.
- [x] Document operational constraints (timeouts, concurrency, retention).

## 10) Post-MVP Enhancements
- [ ] Build cache keyed by repo+ref+env.
- [ ] Build history and persistent metadata (SQLite/Postgres).
- [ ] Optional auth and per-user quotas.
- [ ] Optional WebSocket mode for bi-directional control (cancel/restart).
