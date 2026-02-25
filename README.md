# Meshtastic Firmware Builder

Web application for building Meshtastic firmware from non-official fork repositories.

User flow:
1. User provides repository URL.
2. Frontend loads default branch, recent branches, and recent tags (manual ref input still available).
3. User solves captcha to discover devices, then solves captcha again before build start.
4. Backend clones repository and runs `pio run -e <target>` inside Docker container.
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

Builder image includes build accelerators:
- `mklittlefs` tool preinstalled;
- `ccache` enabled for common embedded GCC toolchains;
- `PLATFORMIO_BUILD_CACHE_DIR` enabled inside persistent PlatformIO cache.

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

## All-in-One Docker Mode

Single Docker image containing backend, frontend, and Nginx reverse proxy. Supports multi-architecture (amd64 + arm64).

### 1) Build PlatformIO builder image (required for builds)

```bash
make builder-image
```

### 2) Build all-in-one image

Build for both architectures:

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t meshtastic-firmware-builder:latest .
```

Build for current architecture only:

```bash
docker build -t meshtastic-firmware-builder:latest .
```

### 3) Run container

```bash
docker run -d \
  --name meshtastic-builder \
  -p 80:80 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v ./build-workdir:/app/build-workdir \
  -e APP_CONCURRENT_BUILDS=1 \
  -e APP_DOCKER_HOST_WORKDIR=/app/build-workdir \
  meshtastic-firmware-builder:latest
```

Access the application at `http://localhost`.

**Why this works:**
- **Port 80**: Nginx serves frontend (React) and proxies `/api/*` to backend (port 8080 internally)
- **Docker socket**: Backend launches PlatformIO builder containers for firmware builds
- **build-workdir volume**: Persisted storage for cloned repositories, build artifacts, and PlatformIO cache

### 4) Configure via environment variables

Override defaults:

```bash
docker run -d \
  --name meshtastic-builder \
  -p 80:80 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v ./build-workdir:/app/build-workdir \
  -e APP_CONCURRENT_BUILDS=2 \
  -e APP_BUILD_TIMEOUT_MINUTES=120 \
  -e APP_RETENTION_HOURS=24 \
  meshtastic-firmware-builder:latest
```

See [Configuration](#configuration) for all available variables.

### 5) View logs

```bash
# All logs
docker logs -f meshtastic-builder

# Specific service logs (supervisor-managed)
docker exec meshtastic-builder cat /var/log/supervisor/backend-stdout.log
docker exec meshtastic-builder cat /var/log/supervisor/nginx-stderr.log
```

### Architecture Decisions

**Multi-stage build:**
- **Backend builder**: Go 1.26 Alpine → minimal static binary
- **Frontend builder**: Node 22 Alpine → production-optimized Vite build
- **Final image**: Alpine Linux + Nginx + supervisord → smallest footprint

**Why separate PlatformIO builder?**
- PlatformIO requires heavy dependencies (Python 3.12, build-essential, PlatformIO CLI)
- Builder runs in isolated containers via Docker socket (backend orchestrates)
- Separation keeps main image small and secure (~50MB vs ~500MB with PlatformIO)

**Why supervisord?**
- Single container runs both Nginx (frontend/reverse proxy) and Go backend
- Simplifies deployment: one container instead of two with docker-compose
- Automatic restart of failed services

## Docker Mode (Separate Containers)

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
  - Body: `{ "repoUrl": "...", "ref": "main", "captchaId": "...", "captchaAnswer": "..." }`
  - Returns build targets discovered from `[env:*]` sections in `variants/**/platformio.ini`
- `POST /api/repos/refs`
  - Body: `{ "repoUrl": "..." }`
  - Returns `defaultBranch`, recent branches, and recent tags for UI ref picker
- `GET /api/captcha`
  - Returns one-time captcha challenge (`captchaId`, `question`, `expiresAt`)
- `POST /api/jobs`
  - Body (first build in browser session): `{ "repoUrl": "...", "ref": "main", "device": "tbeam", "captchaId": "...", "captchaAnswer": "..." }`
  - Body (next builds in same browser session): `{ "repoUrl": "...", "ref": "main", "device": "tbeam", "captchaSessionToken": "..." }`
  - Creates build job
- `GET /api/jobs/{jobId}`
  - Returns current status (`queued|running|success|failed|cancelled`)
  - For queued jobs, response may include `queuePosition` (1-based) and `queueEtaSeconds` (approximate wait time)
- `GET /api/jobs/{jobId}/logs`
  - Returns current log snapshot
- `GET /api/jobs/{jobId}/logs/stream`
  - SSE stream with live log lines
- `GET /api/jobs/{jobId}/artifacts`
  - Returns firmware files found in `.pio/build/<target>/` (`.bin`, `.hex`, `.uf2`, `.elf`)
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
- `APP_PLATFORMIO_CACHE_DIR=./build-workdir/platformio-cache`
- `APP_DOCKER_HOST_WORKDIR=/absolute/path/.../build-workdir` (required for Dockerized backend)
- `APP_DOCKER_HOST_CACHE_DIR=/absolute/path/.../build-workdir/platformio-cache` (recommended)

Build speed notes:
- Backend runs builds with `PLATFORMIO_BUILD_CACHE_DIR=/root/.platformio/build-cache`.
- `ccache` cache and PlatformIO cache both live under mounted `/root/.platformio`, so repeated builds are significantly faster.
- Git submodules are updated in parallel (`--jobs 8`) with compatibility fallback.

## Security notes

- Repository URL, ref, and device names are validated on API boundary
- Build command is executed without shell interpolation (no string shell execution)
- Workspaces are isolated per job under `build-workdir/jobs/{jobId}`
- Artifacts are served only from files registered for that job
- Build creation endpoint has per-client in-memory rate limiting
- Build creation endpoint requires captcha once per browser session (`captchaSessionToken` for subsequent builds)

## Testing

Run backend tests:

```bash
make backend-test
```
