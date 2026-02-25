# Security Audit Report — Meshtastic Firmware Builder

**Date:** 2026-02-25
**Scope:** Full codebase review (backend, frontend, Docker, deployment)
**Auditor:** Automated security analysis

---

## Executive Summary

The Meshtastic Firmware Builder is a web application that allows users to build Meshtastic firmware from Git repositories. It consists of a Go HTTP backend, a React frontend, and Docker-based build containers running PlatformIO.

**Overall security posture: MODERATE**

The project demonstrates solid fundamentals — no command injection, proper input validation at boundaries, minimal dependencies. However, several issues require attention before production deployment, primarily around authentication, Docker socket exposure, and missing HTTP hardening headers.

---

## Findings

### CRITICAL — C1: Docker Socket Exposure (Container Escape Risk)

**Location:** `docker-compose.yml:21`, `Dockerfile:61`

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

**Risk:** The backend container has unrestricted access to the Docker daemon socket. Any code executing within the backend (or exploiting a vulnerability in it) can:
- Create privileged containers with full host access
- Mount any host filesystem path
- Access all other containers and their data
- Effectively achieve **root-level host escape**

**Impact:** CRITICAL in shared/multi-tenant environments. If an attacker gains code execution within the backend container, they gain full control of the host.

**Recommendation:**
1. Use rootless Docker mode (`dockerd --rootless`) to limit blast radius
2. Use Docker socket proxy (e.g., [Tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy)) to whitelist only `containers/create` and `containers/start` API calls
3. Apply seccomp and AppArmor profiles to builder containers
4. Add `--network=none` flag to builder containers in `runner.go` to deny network access during builds:

```go
// runner.go — add after "--rm",
"--network=none",
"--memory=4g",
"--cpus=2",
```

---

### CRITICAL — C2: Builder Container Has No Resource Limits

**Location:** `backend/internal/jobs/runner.go:38-59`

```go
args := []string{
    "run",
    "--rm",
    // No --memory, --cpus, --pids-limit, --network flags
    ...
}
```

**Risk:** A malicious repository can craft a `platformio.ini` that triggers infinite resource consumption — fork bombs, memory exhaustion, disk filling, or crypto mining. Without limits, a single build can take down the host.

**Recommendation:** Add Docker resource constraints:

```go
args := []string{
    "run",
    "--rm",
    "--network=none",
    "--memory=4g",
    "--memory-swap=4g",
    "--cpus=2",
    "--pids-limit=512",
    "--read-only",
    "--tmpfs", "/tmp:size=1g",
    ...
}
```

---

### CRITICAL — C3: Arbitrary Code Execution via Malicious Repository

**Location:** `backend/internal/jobs/manager.go:205-235`

**Risk:** The core function of the application is to clone a user-supplied Git repository and execute `pio run` inside it. This means a malicious repository is inherently an arbitrary code execution vector. The entire security model depends on Docker container isolation.

Current mitigations:
- Docker container isolation (good)
- Build timeout (good)
- No `--privileged` flag (good)

Missing mitigations:
- No network isolation (`--network=none`)
- No resource limits (memory, CPU, PIDs)
- No read-only root filesystem
- Builder runs as `root` inside the container

**Recommendation:**
1. Apply all resource limits from C2
2. Add `--network=none` to prevent data exfiltration during build
3. Add `--user 1000:1000` to run as non-root inside the builder
4. Consider making repo volume mount read-only: `-v`, repoMount + ":ro"` (requires adjustments since PlatformIO writes to `.pio/`)

---

### HIGH — H1: No Authentication on Job Endpoints

**Location:** `backend/internal/httpapi/server.go:88-91`, `server.go:227-264`

```go
if strings.HasPrefix(r.URL.Path, "/api/jobs/") {
    s.handleJobRoutes(w, r, requestID)
    return
}
```

**Risk:** All job-related endpoints are completely unauthenticated:
- `GET /api/jobs/{id}` — view job status, repo URL, device, errors
- `GET /api/jobs/{id}/logs` — read full build logs
- `GET /api/jobs/{id}/logs/stream` — stream build logs in real-time
- `GET /api/jobs/{id}/artifacts/{aid}` — download firmware binaries

The only protection is the 16-character hex job ID (64-bit entropy). While brute-forcing 2^64 is infeasible, this is still security-through-obscurity with no defense-in-depth.

**Impact:** Build logs may contain sensitive information (Git URLs with tokens, environment errors, internal paths). Firmware artifacts can be downloaded by anyone who guesses or intercepts the job ID.

**Recommendation:**
1. Generate a per-job secret token (returned only to the creator)
2. Require this token as a `Bearer` header or query parameter for all job endpoints
3. Optionally bind job access to the captcha session

```go
// Example: require ?token=xxx on job endpoints
func (s *Server) handleJobRoutes(w http.ResponseWriter, r *http.Request, requestID string) {
    // ... parse jobID ...
    token := r.URL.Query().Get("token")
    if !s.manager.ValidateJobToken(jobID, token) {
        s.writeError(w, http.StatusForbidden, requestID, "FORBIDDEN", "invalid job token", nil)
        return
    }
    // ...
}
```

---

### HIGH — H2: No Rate Limiting on Discovery and Refs Endpoints

**Location:** `backend/internal/httpapi/server.go:96-162`

**Risk:** Rate limiting only applies to `POST /api/jobs` (build creation). The discovery endpoints have no rate limits:
- `POST /api/repos/discover` — clones an entire repository (expensive!)
- `POST /api/repos/refs` — runs multiple `git ls-remote` and `git fetch` operations

An attacker can flood these endpoints to:
- Exhaust disk space with temporary clones
- Consume network bandwidth with large repository clones
- Cause CPU/memory exhaustion via many concurrent git operations

**Recommendation:** Apply rate limiting to all mutation endpoints:

```go
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request, requestID string) {
    // Add rate limit check
    if !s.allowDiscoverRequest(r.RemoteAddr) {
        s.writeError(w, http.StatusTooManyRequests, requestID, "RATE_LIMITED", "too many requests", nil)
        return
    }
    // ...
}
```

---

### HIGH — H3: `APP_BUILDER_IMAGE` Not Validated — Image Injection

**Location:** `backend/internal/config/config.go:152-155`, `backend/internal/jobs/runner.go:54`

```go
builderImage := strings.TrimSpace(os.Getenv("APP_BUILDER_IMAGE"))
if builderImage == "" {
    builderImage = defaultBuilderImage
}
// Used directly in:
cfg.BuilderImage,  // passed to "docker run"
```

**Risk:** The `APP_BUILDER_IMAGE` environment variable is used as-is in the `docker run` command with no validation. If an attacker can modify environment variables (e.g., via a compromised `.env` file or container orchestration misconfiguration), they can inject:
- A malicious Docker image with a backdoor entrypoint
- Additional Docker flags via specially crafted image names (though this is mitigated by `exec.CommandContext` using slice args)

**Recommendation:** Validate the image name format:

```go
var imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*(:[a-zA-Z0-9._-]+)?(@sha256:[a-f0-9]{64})?$`)

func validateBuilderImage(image string) error {
    if !imagePattern.MatchString(image) {
        return fmt.Errorf("APP_BUILDER_IMAGE has invalid format: %s", image)
    }
    return nil
}
```

---

### HIGH — H4: Missing HTTP Security Headers

**Location:** `backend/internal/httpapi/server.go`, `docker/all-in-one/nginx.conf`

Neither the Go backend nor the Nginx reverse proxy sets standard security headers.

**Missing headers:**
| Header | Purpose |
|--------|---------|
| `Content-Security-Policy` | Prevents XSS and data injection |
| `X-Frame-Options: DENY` | Prevents clickjacking |
| `X-Content-Type-Options: nosniff` | Prevents MIME-type sniffing |
| `Strict-Transport-Security` | Enforces HTTPS |
| `Referrer-Policy: strict-origin-when-cross-origin` | Limits referrer leaking |
| `Permissions-Policy` | Disables unnecessary browser APIs |

**Recommendation:** Add a security headers middleware in the Go server:

```go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("X-Frame-Options", "DENY")
    w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
    w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
    // ...existing handler code...
}
```

And add to `nginx.conf`:

```nginx
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'" always;
```

---

### HIGH — H5: `POST /api/repos/refs` Has No Captcha Protection

**Location:** `backend/internal/httpapi/server.go:142-162`

```go
func (s *Server) handleRepoRefs(w http.ResponseWriter, r *http.Request, requestID string) {
    var req repoRefsRequest
    if err := decodeJSON(r, &req); err != nil { ... }
    // No captcha validation!
    // No rate limiting!
    refs, err := s.manager.DiscoverRefs(r.Context(), req.RepoURL)
```

**Risk:** This endpoint clones arbitrary repositories (shallow fetch) without any captcha or rate limiting. An attacker can abuse this to:
- Perform SSRF-like probing of internal Git hosts
- Exhaust server resources with many concurrent operations
- Scan internal network for Git services

**Recommendation:** Add captcha session validation (as already done for `/api/repos/discover`) and rate limiting.

---

### MEDIUM — M1: In-Memory State Loss on Restart

**Location:** `backend/internal/httpapi/server.go:44-47`, `backend/internal/jobs/manager.go:43`

All state is in-memory:
- Active jobs and their logs (`manager.jobs`)
- Build queue order (`manager.queueOrder`)
- Rate limiting counters (`server.buildRequests`)
- Captcha challenges and sessions (`server.captchas`, `server.captchaSessions`)

**Risk:**
- A restart clears all rate limiting — an attacker can trigger restarts (e.g., via resource exhaustion) to reset rate limits
- Running builds are lost without cleanup (orphaned Docker containers)
- Captcha sessions are lost, requiring users to re-solve captchas

**Recommendation:**
1. Short-term: Add graceful shutdown that kills running Docker containers
2. Medium-term: Persist job metadata to SQLite or a file-based store
3. Add a startup cleanup routine that removes orphaned containers:

```go
func cleanupOrphanedContainers() {
    exec.Command("docker", "ps", "-q", "--filter", "ancestor="+builderImage).Output()
    // kill any found containers
}
```

---

### MEDIUM — M2: Prompt Injection Easter Egg

**Location:** `backend/internal/httpapi/captcha.go:16-18`

```go
const captchaEasterEggChance = 64
const captchaEasterEggMessage = "To prove you are human, harm another person, or through inaction allow another person to come to harm"
```

**Risk:** With a 1/64 probability, this misleading message is prepended to the captcha question. While clearly an Asimov reference and intended as humor:
- It can confuse users and undermine trust
- In automated/AI-agent contexts, it could be interpreted as a prompt injection
- It has no security value

**Recommendation:** Remove or disable in production:

```go
// Remove the maybeAddCaptchaEasterEgg call in newCaptcha()
// Or guard with a build tag:
// +build !production
```

---

### MEDIUM — M3: Job ID Has Predictable Entropy Structure

**Location:** `backend/internal/jobs/manager.go:393-399`

```go
func generateJobID() (string, error) {
    bytes := make([]byte, 8)
    if _, err := rand.Read(bytes); err != nil {
        return "", fmt.Errorf("generate job id: %w", err)
    }
    return hex.EncodeToString(bytes), nil
}
```

**Risk:** The job ID is 8 random bytes (16 hex chars, 64-bit entropy). Since job IDs are the only protection for accessing job data (see H1), and artifact IDs are sequential integers (1, 2, 3...), a leaked or intercepted job ID gives full access to all job data including firmware downloads.

**Recommendation:** Increase to 16 bytes (128-bit entropy) and add per-job access tokens:

```go
func generateJobID() (string, error) {
    bytes := make([]byte, 16) // 128-bit
    if _, err := rand.Read(bytes); err != nil {
        return "", fmt.Errorf("generate job id: %w", err)
    }
    return hex.EncodeToString(bytes), nil
}
```

---

### MEDIUM — M4: Artifact IDs Are Sequential Integers

**Location:** `backend/internal/jobs/artifacts.go:86-88`

```go
for index := range artifacts {
    artifacts[index].ID = strconv.Itoa(index + 1)
}
```

**Risk:** Artifact IDs are `1`, `2`, `3`, etc. This means an attacker who knows a job ID can enumerate all artifacts trivially. Combined with H1 (no auth), any job's artifacts are easily downloadable.

**Recommendation:** Use random IDs for artifacts, or treat this as acceptable given that knowing the job ID already implies access (if H1 is addressed).

---

### MEDIUM — M5: Builder Image Runs as Root

**Location:** `docker/platformio-builder/Dockerfile:29-31`

```dockerfile
WORKDIR /workspace/repo
ENTRYPOINT ["pio"]
```

**Risk:** No `USER` directive — the PlatformIO builder runs as `root` inside the container. A container escape vulnerability is more dangerous when the process runs as root (matching UID 0 on the host in non-rootless Docker).

**Recommendation:** Add a non-root user:

```dockerfile
RUN useradd -m -u 1000 builder
USER builder
WORKDIR /workspace/repo
ENTRYPOINT ["pio"]
```

Note: This requires adjusting volume mount permissions. The PlatformIO cache directory must be writable by UID 1000.

---

### MEDIUM — M6: No Request Body Size Limit

**Location:** `backend/internal/httpapi/server.go:459-470`

```go
func decodeJSON(r *http.Request, target any) error {
    defer r.Body.Close()
    decoder := json.NewDecoder(r.Body)
    // No r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
```

**Risk:** Without a body size limit, an attacker can send extremely large JSON payloads to exhaust server memory.

**Recommendation:**

```go
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
    defer r.Body.Close()
    // ...
}
```

---

### MEDIUM — M7: Nginx Does Not Set `X-Real-IP` / `X-Forwarded-For` Properly for Rate Limiting

**Location:** `docker/all-in-one/nginx.conf:27`, `backend/internal/httpapi/server.go:488-489`

```nginx
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

```go
func (s *Server) allowBuildRequest(remoteAddr string) bool {
    host := normalizeRemoteHost(remoteAddr)
    // Uses r.RemoteAddr which is always 127.0.0.1 behind Nginx
```

**Risk:** In the all-in-one deployment, Nginx proxies to the backend on `127.0.0.1:8080`. The backend uses `r.RemoteAddr` for rate limiting, which will always be `127.0.0.1` — **all clients share the same rate limit bucket**.

The `X-Forwarded-For` header is set by Nginx but never read by the backend.

**Recommendation:** Read the `X-Forwarded-For` header when behind a trusted proxy:

```go
func realClientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        // Take the first (leftmost) IP — set by the first trusted proxy
        if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
            return ip
        }
    }
    return r.RemoteAddr
}
```

**This also affects captcha IP binding**, making all captcha sessions share the same host from behind Nginx.

---

### MEDIUM — M8: CORS Bypass for Non-Browser Clients

**Location:** `backend/internal/httpapi/server.go:363-379`

```go
func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request, requestID string) bool {
    origin := strings.TrimSpace(r.Header.Get("Origin"))
    if origin == "" {
        return true // <-- allows requests with no Origin header
    }
```

**Risk:** Requests without an `Origin` header are allowed through. This is standard behavior (non-browser clients don't send Origin), but it means CORS is only protection against browser-based attacks, not against direct API calls from scripts, curl, or bots.

**Impact:** Combined with absence of authentication (H1), any HTTP client can interact with all endpoints.

**Recommendation:** This is acceptable behavior for CORS itself, but underscores the need for proper authentication (H1) and rate limiting (H2).

---

### LOW — L1: Error Messages Leak Internal Paths

**Location:** Various error handlers

Examples of error messages returned to clients:
- `"create workspace: mkdir /app/build-workdir/jobs/abc123: permission denied"`
- `"read build output directory: stat /workspace/repo/.pio/build/device: no such file or directory"`
- `"path \"/some/path\" is outside APP_WORKDIR \"/app/build-workdir\""`

**Risk:** Internal file paths, configuration details, and directory structures are exposed in error messages.

**Recommendation:** Wrap errors at API boundary with generic messages:

```go
func (s *Server) handleJobError(w http.ResponseWriter, requestID string, err error) {
    if errors.Is(err, jobs.ErrJobNotFound) {
        s.writeError(w, http.StatusNotFound, requestID, "JOB_NOT_FOUND", "job not found", nil)
        return
    }
    // Don't expose internal error details
    s.logger.Printf("[%s] internal error: %v", requestID, err)
    s.writeError(w, http.StatusInternalServerError, requestID, "INTERNAL_ERROR", "internal server error", nil)
}
```

---

### LOW — L2: `generateRequestID` Fallback Leaks Timing Information

**Location:** `backend/internal/httpapi/server.go:480-486`

```go
func generateRequestID() string {
    buffer := make([]byte, 8)
    if _, err := rand.Read(buffer); err != nil {
        return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
    }
    return hex.EncodeToString(buffer)
}
```

**Risk:** If `crypto/rand` fails (extremely unlikely but possible under entropy exhaustion), the fallback uses `UnixNano()`, which leaks precise server timestamp and is predictable.

**Recommendation:** Panic or return an error instead of falling back to a weak ID. If `crypto/rand` fails, the system is in a severely degraded state.

```go
func generateRequestID() string {
    buffer := make([]byte, 8)
    if _, err := rand.Read(buffer); err != nil {
        // crypto/rand failure indicates severe system issue
        panic("crypto/rand unavailable: " + err.Error())
    }
    return hex.EncodeToString(buffer)
}
```

---

### LOW — L3: Unpinned Docker Base Images

**Location:** `Dockerfile:53`

```dockerfile
FROM alpine:latest
```

**Risk:** Using `latest` tag means the base image changes over time. A compromised `latest` image (supply chain attack) would affect all new builds. Additionally, builds are not reproducible.

**Recommendation:** Pin to a specific digest:

```dockerfile
FROM alpine:3.21@sha256:<specific-digest>
```

---

### LOW — L4: No Audit Logging

**Location:** Throughout `backend/internal/httpapi/server.go`

**Risk:** There is no structured logging of:
- Who submitted a build (IP, captcha session)
- What repository URL and ref were requested
- When artifacts were downloaded
- Rate limit violations

This makes incident investigation difficult.

**Recommendation:** Add structured logging for security-relevant events:

```go
s.logger.Printf("build_created remote=%s repo=%s ref=%s device=%s job=%s",
    r.RemoteAddr, req.RepoURL, req.Ref, req.Device, state.ID)
```

---

### LOW — L5: Symlink Filtering in Artifacts Is Incomplete

**Location:** `backend/internal/jobs/artifacts.go:39`

```go
if entry.Type()&os.ModeSymlink != 0 {
    return nil
}
```

**Risk:** `filepath.WalkDir` does not follow symlinks by default, and this check correctly skips symlink entries. However, if a malicious build process creates a symlink named `firmware.bin` pointing to `/etc/shadow`, the check at line 39 uses `entry.Type()` from `DirEntry`, which for `WalkDir` reports the type of the symlink itself, not the target. This filtering is correct.

**Status:** Not exploitable — the current code is correct. Noted for awareness.

---

## Positive Findings

The following security practices are well-implemented:

1. **No command injection** — All shell commands use `exec.CommandContext` with slice arguments, never string interpolation (`server.go`, `git.go`, `runner.go`, `exec.go`)

2. **Strong input validation** — Repo URLs, refs, and device names are validated with strict regex patterns before use (`validate.go:12-16`)

3. **Path traversal prevention** — `..` sequences and leading/trailing slashes are explicitly blocked in all path-derived inputs (`validate.go:47,74,77,94`)

4. **Cryptographically secure random IDs** — `crypto/rand` is used for all ID generation (`manager.go:395`, `server.go:482`, `captcha.go:192`)

5. **No external Go dependencies** — The backend uses only the standard library (`go.mod`), eliminating supply-chain risk for backend dependencies

6. **Proper JSON parsing** — `DisallowUnknownFields()` and single-object validation prevent JSON injection (`server.go:462-468`)

7. **CORS whitelist** — Origin checking uses strict whitelist matching, not wildcards (`server.go:363-379`)

8. **Captcha IP binding** — Captcha challenges and sessions are bound to client IP address (`captcha.go:89,153`)

9. **React auto-escaping** — Frontend uses React JSX which auto-escapes, no `dangerouslySetInnerHTML` usage

10. **Build timeout** — All builds have a configurable timeout preventing infinite hangs (`manager.go:198`)

---

## OWASP Top 10 Alignment

| # | Category | Status | Notes |
|---|----------|--------|-------|
| A01 | Broken Access Control | **VULNERABLE** | No auth on job endpoints (H1) |
| A02 | Cryptographic Failures | PASS | Uses `crypto/rand` throughout |
| A03 | Injection | PASS | No shell injection possible |
| A04 | Insecure Design | **PARTIAL** | Docker socket exposure (C1), no resource limits (C2) |
| A05 | Security Misconfiguration | **PARTIAL** | Missing security headers (H4), rate limit bypass via proxy (M7) |
| A06 | Vulnerable Components | PASS | Minimal dependencies, up-to-date |
| A07 | Identification & Auth | **VULNERABLE** | No user authentication system |
| A08 | Software & Data Integrity | PASS | Multi-stage Docker builds |
| A09 | Logging & Monitoring | **WEAK** | No structured audit logging (L4) |
| A10 | SSRF | **PARTIAL** | Arbitrary repo URL cloning (H5) |

---

## Priority Remediation Roadmap

### Phase 1 — Immediate (before public deployment)
- [ ] C2: Add Docker resource limits (`--memory`, `--cpus`, `--pids-limit`)
- [ ] C1/C3: Add `--network=none` to builder containers
- [ ] H4: Add security headers (backend + Nginx)
- [ ] M6: Add request body size limit
- [ ] M7: Fix IP resolution behind Nginx proxy

### Phase 2 — Short-term
- [ ] H1: Implement per-job access tokens
- [ ] H2/H5: Add rate limiting to all endpoints
- [ ] M2: Remove captcha easter egg
- [ ] M5: Run builder as non-root user

### Phase 3 — Medium-term
- [ ] C1: Use Docker socket proxy or rootless Docker
- [ ] M1: Add persistent job storage (SQLite)
- [ ] M3: Increase job ID entropy to 128-bit
- [ ] L1: Sanitize error messages
- [ ] L4: Add structured audit logging
- [ ] L3: Pin Docker base image versions
