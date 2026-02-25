export type JobStatus = "queued" | "running" | "success" | "failed" | "cancelled";

export interface ArtifactItem {
  id: string;
  name: string;
  relativePath: string;
  size: number;
  downloadUrl: string;
}

export interface JobState {
  id: string;
  repoUrl: string;
  ref?: string;
  device: string;
  status: JobStatus;
  captchaSessionToken?: string;
  queuePosition?: number;
  queueEtaSeconds?: number;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  error?: string;
  logLines: number;
  artifacts: ArtifactItem[];
}

export interface DiscoverResponse {
  repoUrl: string;
  ref?: string;
  devices: string[];
  captchaSessionToken?: string;
}

export interface RepoRefItem {
  name: string;
  commit?: string;
  updatedAt?: string;
}

export interface RepoRefsResponse {
  repoUrl: string;
  defaultBranch?: string;
  recentBranches: RepoRefItem[];
  recentTags: RepoRefItem[];
}

export interface CaptchaChallenge {
  captchaRequired?: boolean;
  captchaId?: string;
  question?: string;
  expiresAt?: string;
}

export interface ServerHealth {
  status: string;
  captchaRequired: boolean;
}

export interface LogsSnapshot {
  lines: string[];
}

const DEFAULT_API_BASE_URL = "http://localhost:8080";
const API_BASE_URLS = resolveApiBaseUrls(import.meta.env.VITE_API_BASE_URLS, import.meta.env.VITE_API_BASE_URL);
const BACKEND_HEALTH_CHECK_PATH = "/api/healthz";
const BACKEND_HEALTH_CACHE_TTL_MS = 5000;
const BACKEND_UNAVAILABLE_COOLDOWN_MS = 15000;
const BACKEND_HEALTH_TIMEOUT_MS = 1500;
const BACKEND_UNAVAILABLE_STATUSES = new Set<number>([502, 503, 504]);

interface BackendHealthState {
  healthy: boolean;
  checkedAtMs: number;
  unavailableUntilMs: number;
}

type BackendResolvedHandler = (backendBaseUrl: string) => void;

let nextApiBaseUrlIndex = 0;

const backendHealthStates = new Map<string, BackendHealthState>(
  API_BASE_URLS.map((backendBaseUrl) => [
    backendBaseUrl,
    {
      healthy: true,
      checkedAtMs: 0,
      unavailableUntilMs: 0,
    },
  ]),
);

export function pickApiBaseUrl(): string {
  const roundRobinOrder = buildRoundRobinOrder();
  for (const backendBaseUrl of roundRobinOrder) {
    if (!isBackendInCooldown(backendBaseUrl)) {
      return backendBaseUrl;
    }
  }

  return roundRobinOrder[0];
}

export function apiUrl(path: string, backendBaseUrl?: string): string {
  return `${resolveApiBaseUrl(backendBaseUrl)}${path}`;
}

export async function discoverDevices(
  repoUrl: string,
  ref: string,
  captchaId?: string,
  captchaAnswer?: string,
  captchaSessionToken?: string,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<DiscoverResponse> {
  const payload: Record<string, string> = { repoUrl, ref };
  if (captchaId) {
    payload.captchaId = captchaId;
  }
  if (captchaAnswer) {
    payload.captchaAnswer = captchaAnswer;
  }
  if (captchaSessionToken) {
    payload.captchaSessionToken = captchaSessionToken;
  }

  return request<DiscoverResponse>("/api/repos/discover", {
    method: "POST",
    body: JSON.stringify(payload),
    backendBaseUrl,
    onBackendResolved,
  });
}

export async function discoverRepoRefs(
  repoUrl: string,
  signal?: AbortSignal,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<RepoRefsResponse> {
  return request<RepoRefsResponse>("/api/repos/refs", {
    method: "POST",
    body: JSON.stringify({ repoUrl }),
    signal,
    backendBaseUrl,
    onBackendResolved,
  });
}

export async function createBuildJob(
  repoUrl: string,
  ref: string,
  device: string,
  captchaId?: string,
  captchaAnswer?: string,
  captchaSessionToken?: string,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<JobState> {
  const payload: Record<string, string> = { repoUrl, ref, device };
  if (captchaId) {
    payload.captchaId = captchaId;
  }
  if (captchaAnswer) {
    payload.captchaAnswer = captchaAnswer;
  }
  if (captchaSessionToken) {
    payload.captchaSessionToken = captchaSessionToken;
  }

  return request<JobState>("/api/jobs", {
    method: "POST",
    body: JSON.stringify(payload),
    backendBaseUrl,
    onBackendResolved,
  });
}

export async function getCaptchaChallenge(
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<CaptchaChallenge> {
  return request<CaptchaChallenge>("/api/captcha", {
    method: "GET",
    backendBaseUrl,
    onBackendResolved,
  });
}

export async function getServerHealth(
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<ServerHealth> {
  return request<ServerHealth>("/api/healthz", {
    method: "GET",
    backendBaseUrl,
    onBackendResolved,
  });
}

export async function getJob(
  jobId: string,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<JobState> {
  return request<JobState>(`/api/jobs/${jobId}`, { backendBaseUrl, onBackendResolved });
}

export async function getLogs(
  jobId: string,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<LogsSnapshot> {
  return request<LogsSnapshot>(`/api/jobs/${jobId}/logs`, { backendBaseUrl, onBackendResolved });
}

export async function getArtifacts(
  jobId: string,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): Promise<ArtifactItem[]> {
  const response = await request<{ artifacts: ArtifactItem[] }>(`/api/jobs/${jobId}/artifacts`, {
    backendBaseUrl,
    onBackendResolved,
  });
  return response.artifacts;
}

export function createLogStream(
  jobId: string,
  backendBaseUrl?: string,
  onBackendResolved?: BackendResolvedHandler,
): EventSource {
  const resolvedBackendBaseUrl = resolveApiBaseUrl(backendBaseUrl ?? pickApiBaseUrl());
  onBackendResolved?.(resolvedBackendBaseUrl);
  return new EventSource(apiUrl(`/api/jobs/${jobId}/logs/stream`, resolvedBackendBaseUrl));
}

interface ApiRequestInit extends RequestInit {
  backendBaseUrl?: string;
  onBackendResolved?: BackendResolvedHandler;
}

async function request<T>(path: string, init?: ApiRequestInit): Promise<T> {
  const onBackendResolved = init?.onBackendResolved;
  const requestInit: RequestInit = { ...init };
  delete (requestInit as { backendBaseUrl?: string }).backendBaseUrl;
  delete (requestInit as { onBackendResolved?: BackendResolvedHandler }).onBackendResolved;

  const attemptOrder = buildBackendAttemptOrder(init?.backendBaseUrl);
  let hasUnavailableBackend = false;

  for (const backendBaseUrl of attemptOrder) {
    throwIfAborted(requestInit.signal);

    const backendAvailable = await isBackendAvailable(backendBaseUrl, path);
    if (!backendAvailable) {
      hasUnavailableBackend = true;
      continue;
    }

    let response: Response;
    try {
      response = await fetch(apiUrl(path, backendBaseUrl), {
        headers: {
          "Content-Type": "application/json",
        },
        ...requestInit,
      });
    } catch (requestError) {
      if (isAbortError(requestError)) {
        throw requestError;
      }

      hasUnavailableBackend = true;
      markBackendUnavailable(backendBaseUrl);
      continue;
    }

    if (isBackendUnavailableStatus(response.status)) {
      hasUnavailableBackend = true;
      markBackendUnavailable(backendBaseUrl);
      continue;
    }

    markBackendHealthy(backendBaseUrl);
    onBackendResolved?.(backendBaseUrl);

    const payload = await parseApiPayload<T>(response);
    if (!response.ok) {
      const message = payload.error?.message ?? `Request failed (${response.status})`;
      throw new Error(message);
    }
    if (typeof payload.data === "undefined") {
      throw new Error("Malformed API response");
    }

    return payload.data;
  }

  if (hasUnavailableBackend) {
    throw new Error("All backend nodes are unavailable");
  }

  throw new Error("Unable to select backend for request");
}

function stripTrailingSlash(value: string): string {
  if (value.endsWith("/")) {
    return value.slice(0, -1);
  }
  return value;
}

function buildBackendAttemptOrder(preferredBackendBaseUrl?: string): string[] {
  const roundRobinOrder = buildRoundRobinOrder();
  const preferredBackend = preferredBackendBaseUrl ? parseApiBaseUrl(preferredBackendBaseUrl) : null;
  if (!preferredBackend) {
    return roundRobinOrder;
  }

  return [preferredBackend, ...roundRobinOrder.filter((backendBaseUrl) => backendBaseUrl !== preferredBackend)];
}

function buildRoundRobinOrder(): string[] {
  const startIndex = nextApiBaseUrlIndex % API_BASE_URLS.length;
  nextApiBaseUrlIndex = (startIndex + 1) % API_BASE_URLS.length;

  const ordered: string[] = [];
  for (let offset = 0; offset < API_BASE_URLS.length; offset += 1) {
    ordered.push(API_BASE_URLS[(startIndex + offset) % API_BASE_URLS.length]);
  }
  return ordered;
}

async function isBackendAvailable(backendBaseUrl: string, requestPath: string): Promise<boolean> {
  if (requestPath === BACKEND_HEALTH_CHECK_PATH) {
    return true;
  }

  const backendState = getBackendHealthState(backendBaseUrl);
  const nowMs = Date.now();

  if (backendState.unavailableUntilMs > nowMs) {
    return false;
  }

  if (backendState.checkedAtMs > 0 && nowMs - backendState.checkedAtMs < BACKEND_HEALTH_CACHE_TTL_MS) {
    return backendState.healthy;
  }

  const healthy = await probeBackendHealth(backendBaseUrl);
  if (healthy) {
    markBackendHealthy(backendBaseUrl, nowMs);
    return true;
  }

  markBackendUnavailable(backendBaseUrl, nowMs);
  return false;
}

function isBackendInCooldown(backendBaseUrl: string): boolean {
  const backendState = getBackendHealthState(backendBaseUrl);
  return backendState.unavailableUntilMs > Date.now();
}

function getBackendHealthState(backendBaseUrl: string): BackendHealthState {
  const existingState = backendHealthStates.get(backendBaseUrl);
  if (existingState) {
    return existingState;
  }

  const fallbackState: BackendHealthState = {
    healthy: true,
    checkedAtMs: 0,
    unavailableUntilMs: 0,
  };
  backendHealthStates.set(backendBaseUrl, fallbackState);
  return fallbackState;
}

function markBackendHealthy(backendBaseUrl: string, nowMs = Date.now()): void {
  const backendState = getBackendHealthState(backendBaseUrl);
  backendState.healthy = true;
  backendState.checkedAtMs = nowMs;
  backendState.unavailableUntilMs = 0;
}

function markBackendUnavailable(backendBaseUrl: string, nowMs = Date.now()): void {
  const backendState = getBackendHealthState(backendBaseUrl);
  backendState.healthy = false;
  backendState.checkedAtMs = nowMs;
  backendState.unavailableUntilMs = nowMs + BACKEND_UNAVAILABLE_COOLDOWN_MS;
}

async function probeBackendHealth(backendBaseUrl: string): Promise<boolean> {
  const controller = new AbortController();
  const timeoutId = globalThis.setTimeout(() => {
    controller.abort();
  }, BACKEND_HEALTH_TIMEOUT_MS);

  try {
    const response = await fetch(apiUrl(BACKEND_HEALTH_CHECK_PATH, backendBaseUrl), {
      method: "GET",
      signal: controller.signal,
    });

    return response.ok;
  } catch {
    return false;
  } finally {
    globalThis.clearTimeout(timeoutId);
  }
}

function isBackendUnavailableStatus(status: number): boolean {
  return BACKEND_UNAVAILABLE_STATUSES.has(status);
}

function throwIfAborted(signal?: AbortSignal | null): void {
  if (signal?.aborted) {
    throw new DOMException("Request aborted", "AbortError");
  }
}

function isAbortError(value: unknown): value is DOMException {
  return value instanceof DOMException && value.name === "AbortError";
}

async function parseApiPayload<T>(response: Response): Promise<{ data?: T; error?: { message?: string } }> {
  try {
    return (await response.json()) as { data?: T; error?: { message?: string } };
  } catch {
    return {};
  }
}

function resolveApiBaseUrl(backendBaseUrl?: string): string {
  if (!backendBaseUrl) {
    return API_BASE_URLS[0];
  }

  const parsedBaseUrl = parseApiBaseUrl(backendBaseUrl);
  if (!parsedBaseUrl) {
    return API_BASE_URLS[0];
  }

  return parsedBaseUrl;
}

function resolveApiBaseUrls(listValue?: string, fallbackValue?: string): string[] {
  const resolved = new Set<string>();

  if (listValue) {
    for (const candidate of splitApiBaseUrlList(listValue)) {
      const parsedBaseUrl = parseApiBaseUrl(candidate);
      if (parsedBaseUrl) {
        resolved.add(parsedBaseUrl);
      }
    }
  }

  const fallbackBaseUrl = parseApiBaseUrl(fallbackValue ?? "") ?? DEFAULT_API_BASE_URL;
  if (resolved.size === 0) {
    resolved.add(fallbackBaseUrl);
  }

  return Array.from(resolved);
}

function splitApiBaseUrlList(value: string): string[] {
  return value
    .split(/[\s,]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function parseApiBaseUrl(value: string): string | null {
  const trimmedValue = value.trim();
  if (!trimmedValue) {
    return null;
  }

  try {
    const parsed = new URL(trimmedValue);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }
    if (parsed.search || parsed.hash) {
      return null;
    }

    return stripTrailingSlash(parsed.toString());
  } catch {
    return null;
  }
}
