# Meshtastic Firmware Builder

Web application for building Meshtastic firmware from non-official fork repositories.

User flow:
1. User provides repository URL and optional branch/tag/commit.
2. Backend clones repository and lists devices from `variants/`.
3. User selects a device.
4. Backend runs `pio run -e <device>` inside Docker container.
5. Frontend shows live build logs and firmware download links.

## Stack

- Backend: Go (`net/http`) with async build queue (configurable, default 1 worker)
- Frontend: Node.js + React + Vite + TypeScript
- Builder runtime: Docker container with PlatformIO CLI

## Repository layout

- `backend/` - API server, job manager, build orchestrator
- `frontend/` - UI with RU/EN and live log viewer
- `docker/platformio-builder/` - Dockerfile for PlatformIO builder image
- `build-workdir/` - runtime workspace (created automatically, gitignored)

## Quick start (local)

### 1) Build PlatformIO builder image

```bash
make builder-image
```

### 2) Start backend

```bash
cd backend
go run ./cmd/server
```

### 3) Start frontend

```bash
cd frontend
npm install
npm run dev
```

Frontend opens on `http://localhost:5173`, backend on `http://localhost:8080`.

## Docker mode

You can run frontend and backend via Docker:

```bash
docker compose up --build
```

Before the first run, ensure builder image exists:

```bash
make builder-image
```

Notes:
- backend needs Docker socket access to launch build containers;
- `docker-compose.yml` defaults host mount mapping to `${PWD}/build-workdir`; override with `APP_DOCKER_HOST_WORKDIR` and `APP_DOCKER_HOST_CACHE_DIR` when needed.
In Docker Compose mode, frontend proxies `/api/*` to backend internally, so browser CORS issues are avoided.

## API

- `POST /api/repos/discover`
  - Body: `{ "repoUrl": "...", "ref": "main" }`
  - Returns devices discovered from `variants/`
- `POST /api/jobs`
  - Body: `{ "repoUrl": "...", "ref": "main", "device": "tbeam" }`
  - Creates build job
- `GET /api/jobs/{jobId}`
  - Returns current status (`queued|running|success|failed|cancelled`)
- `GET /api/jobs/{jobId}/logs`
  - Returns current log snapshot
- `GET /api/jobs/{jobId}/logs/stream`
  - SSE stream with live log lines
- `GET /api/jobs/{jobId}/artifacts`
  - Returns list of all files found in `.pio/build/<device>/`
- `GET /api/jobs/{jobId}/artifacts/{artifactId}`
  - Downloads artifact file

## Configuration

Use environment variables from `env.sample`.

Important defaults:
- `APP_CONCURRENT_BUILDS=1` (configurable)
- `APP_RETENTION_HOURS=168` (one week)
- `APP_BUILD_TIMEOUT_MINUTES=90`
- `APP_ALLOWED_ORIGINS=http://localhost:5173`
- `APP_BUILD_RATE_LIMIT_PER_MINUTE=10`
- `APP_DOCKER_HOST_WORKDIR=/absolute/path/.../build-workdir` (required for Dockerized backend)
- `APP_DOCKER_HOST_CACHE_DIR=/absolute/path/.../build-workdir/platformio-cache` (recommended)

## Security notes

- Repository URL, ref, and device names are validated on API boundary
- Build command is executed without shell interpolation (no string shell execution)
- Workspaces are isolated per job under `build-workdir/jobs/{jobId}`
- Artifacts are served only from files registered for that job
- Build creation endpoint has per-client in-memory rate limiting

## Testing

Run backend tests:

```bash
make backend-test
```
