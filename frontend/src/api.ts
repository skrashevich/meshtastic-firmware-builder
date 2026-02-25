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
  nodeBaseUrl?: string;
  proxyBackendUrls?: string[];
  runningBuilds: number;
  queuedBuilds: number;
  concurrentBuilds: number;
}

export interface LogsSnapshot {
  lines: string[];
}

export interface BackendRouteInfo {
  backendBaseUrl: string;
  gatewayBaseUrl: string;
  proxied: boolean;
}

export type BackendNodeHealthStatus = "alive" | "degraded";

export interface BackendNodeHealth {
  baseUrl: string;
  status: BackendNodeHealthStatus;
  isCurrentBackend: boolean;
  isCurrentGateway: boolean;
}

const DEFAULT_API_BASE_URL = "http://localhost:8080";
const INITIAL_API_BASE_URLS = resolveApiBaseUrls(import.meta.env.VITE_API_BASE_URLS, import.meta.env.VITE_API_BASE_URL);
const BACKEND_HEALTH_CHECK_PATH = "/api/healthz";
const BACKEND_HEALTH_CACHE_TTL_MS = 5000;
const BACKEND_UNAVAILABLE_COOLDOWN_MS = 15000;
const BACKEND_HEALTH_TIMEOUT_MS = 1500;
const BACKEND_UNAVAILABLE_STATUSES = new Set<number>([502, 503, 504]);
const BACKEND_LOAD_INFO_TTL_MS = 10000;
const TARGET_BACKEND_HEADER = "X-MFB-Target-Backend";
const RESPONSE_SERVED_BY_HEADER = "X-MFB-Served-By";
const RESPONSE_PROXIED_VIA_HEADER = "X-MFB-Proxied-Via";
const TARGET_BACKEND_QUERY_PARAM = "__mfb_target_backend";
const PROXY_TARGET_NOT_ALLOWED_CODE = "PROXY_TARGET_NOT_ALLOWED";

interface BackendHealthState {
  healthy: boolean;
  checkedAtMs: number;
  unavailableUntilMs: number;
}

interface BackendLoadState {
  runningBuilds: number;
  queuedBuilds: number;
  concurrentBuilds: number;
  checkedAtMs: number;
}

type BackendResolvedHandler = (route: BackendRouteInfo) => void;
type BackendHealthListener = () => void;

let nextApiBaseUrlIndex = 0;
let currentGatewayBaseUrl = DEFAULT_API_BASE_URL;

const backendPool: string[] = [];
const backendHealthStates = new Map<string, BackendHealthState>();
const backendLoadStates = new Map<string, BackendLoadState>();

const backendHealthListeners = new Set<BackendHealthListener>();

registerBackendPool(INITIAL_API_BASE_URLS);
if (backendPool.length === 0) {
  registerBackendPool([DEFAULT_API_BASE_URL]);
}
currentGatewayBaseUrl = backendPool[0];

export function pickApiBaseUrl(): string {
  const roundRobinOrder = buildRoundRobinOrder();
  for (const backendBaseUrl of roundRobinOrder) {
    if (!isBackendInCooldown(backendBaseUrl)) {
      return backendBaseUrl;
    }
  }

  return roundRobinOrder[0];
}

export function pickGatewayBaseUrl(): string {
  if (!isBackendInCooldown(currentGatewayBaseUrl)) {
    return resolveApiBaseUrl(currentGatewayBaseUrl);
  }

  const fallbackOrder = getBackendPoolSnapshot().filter(
    (backendBaseUrl) => !isSameBackendNode(backendBaseUrl, currentGatewayBaseUrl),
  );
  for (const backendBaseUrl of fallbackOrder) {
    if (!isBackendInCooldown(backendBaseUrl)) {
      return backendBaseUrl;
    }
  }

  return fallbackOrder[0] ?? resolveApiBaseUrl(undefined);
}

export function apiUrl(path: string, backendBaseUrl?: string): string {
  return `${resolveApiBaseUrl(backendBaseUrl)}${path}`;
}

export function routedApiUrl(path: string, backendBaseUrl?: string, gatewayBaseUrl?: string): string {
  const targetBackendBaseUrl = resolveApiBaseUrl(backendBaseUrl ?? pickApiBaseUrl());
  const resolvedGatewayBaseUrl = resolveApiBaseUrl(gatewayBaseUrl ?? pickGatewayBaseUrl());

  if (isSameBackendNode(targetBackendBaseUrl, resolvedGatewayBaseUrl)) {
    return apiUrl(path, resolvedGatewayBaseUrl);
  }

  return appendTargetBackendQuery(apiUrl(path, resolvedGatewayBaseUrl), targetBackendBaseUrl);
}

export function registerBackendPool(backendBaseUrls: string[]): void {
  let hasChanges = false;

  for (const backendBaseUrl of backendBaseUrls) {
    const normalizedBackendBaseUrl = parseApiBaseUrl(backendBaseUrl ?? "");
    if (!normalizedBackendBaseUrl) {
      continue;
    }

    if (ensureBackendInPool(normalizedBackendBaseUrl)) {
      hasChanges = true;
    }

    const resolvedBackendInPool = findBackendInPool(normalizedBackendBaseUrl) ?? normalizedBackendBaseUrl;
    if (!backendHealthStates.has(resolvedBackendInPool)) {
      backendHealthStates.set(resolvedBackendInPool, {
        healthy: true,
        checkedAtMs: 0,
        unavailableUntilMs: 0,
      });
      hasChanges = true;
    }
  }

  if (hasChanges) {
    emitBackendHealthChanged();
  }
}

export async function refreshBackendLoadSnapshot(preferredBackendBaseUrl?: string): Promise<void> {
  const preferredBackend = parseApiBaseUrl(preferredBackendBaseUrl ?? "");
  if (preferredBackend) {
    ensureBackendInPool(preferredBackend);
  }

  const knownBackends = getBackendPoolSnapshot();
  const candidates =
    preferredBackend === null
      ? knownBackends
      : [preferredBackend, ...knownBackends.filter((backendBaseUrl) => !isSameBackendNode(backendBaseUrl, preferredBackend))];

  await Promise.all(
    candidates.map(async (backendBaseUrl) => {
      try {
        await request<ServerHealth>(BACKEND_HEALTH_CHECK_PATH, {
          method: "GET",
          backendBaseUrl,
        });
      } catch {
        // Ignore probe failures here: regular request path handles failover.
      }
    }),
  );
}

export function subscribeBackendHealth(listener: BackendHealthListener): () => void {
  backendHealthListeners.add(listener);
  return () => {
    backendHealthListeners.delete(listener);
  };
}

export function getBackendNodeHealthSnapshot(
  currentBackendBaseUrl?: string,
  currentGatewayBaseUrl?: string,
): BackendNodeHealth[] {
  const resolvedCurrentBackend = parseApiBaseUrl(currentBackendBaseUrl ?? "") ?? "";
  const resolvedCurrentGateway = parseApiBaseUrl(currentGatewayBaseUrl ?? "") ?? "";
  const knownBackends = getBackendPoolSnapshot();

  return knownBackends.map((baseUrl) => {
    const backendState = getBackendHealthState(baseUrl);
    return {
      baseUrl,
      status: backendState.healthy ? "alive" : "degraded",
      isCurrentBackend: resolvedCurrentBackend !== "" && isSameBackendNode(resolvedCurrentBackend, baseUrl),
      isCurrentGateway: resolvedCurrentGateway !== "" && isSameBackendNode(resolvedCurrentGateway, baseUrl),
    };
  });
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
  const targetBackendBaseUrl = resolveApiBaseUrl(backendBaseUrl ?? pickApiBaseUrl());
  const gatewayBaseUrl = resolveApiBaseUrl(pickGatewayBaseUrl());
  currentGatewayBaseUrl = gatewayBaseUrl;

  const streamUrl =
    isSameBackendNode(targetBackendBaseUrl, gatewayBaseUrl)
      ? apiUrl(`/api/jobs/${jobId}/logs/stream`, gatewayBaseUrl)
      : appendTargetBackendQuery(apiUrl(`/api/jobs/${jobId}/logs/stream`, gatewayBaseUrl), targetBackendBaseUrl);

  onBackendResolved?.({
    backendBaseUrl: targetBackendBaseUrl,
    gatewayBaseUrl,
    proxied: !isSameBackendNode(targetBackendBaseUrl, gatewayBaseUrl),
  });

  return new EventSource(streamUrl);
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

  const method = normalizeHttpMethod(requestInit.method);
  const preferLeastLoaded = shouldPreferLeastLoaded(path, method, init?.backendBaseUrl);
  const targetAttemptOrder = buildBackendAttemptOrder(init?.backendBaseUrl, preferLeastLoaded);
  const gatewayAttemptOrder = buildGatewayAttemptOrder();
  let hasUnavailableBackend = false;

  for (const gatewayBaseUrl of gatewayAttemptOrder) {
    throwIfAborted(requestInit.signal);

    const gatewayAvailable = await isGatewayAvailable(gatewayBaseUrl);
    if (!gatewayAvailable) {
      hasUnavailableBackend = true;
      continue;
    }

    let gatewayTransportFailed = false;

    for (const targetBackendBaseUrl of targetAttemptOrder) {
      throwIfAborted(requestInit.signal);

      if (isBackendInCooldown(targetBackendBaseUrl)) {
        hasUnavailableBackend = true;
        continue;
      }

      const headers = buildRequestHeaders(requestInit.headers, targetBackendBaseUrl, gatewayBaseUrl);

      let response: Response;
      try {
        response = await fetch(apiUrl(path, gatewayBaseUrl), {
          ...requestInit,
          headers,
        });
      } catch (requestError) {
        if (isAbortError(requestError)) {
          throw requestError;
        }

        hasUnavailableBackend = true;
        markBackendUnavailable(gatewayBaseUrl);
        gatewayTransportFailed = true;
        break;
      }

      const payload = await parseApiPayload<T>(response);

      if (
        response.status === 403 &&
        !isSameBackendNode(targetBackendBaseUrl, gatewayBaseUrl) &&
        payload.error?.code === PROXY_TARGET_NOT_ALLOWED_CODE
      ) {
        hasUnavailableBackend = true;
        markBackendUnavailable(targetBackendBaseUrl);
        continue;
      }

      if (isBackendUnavailableStatus(response.status)) {
        hasUnavailableBackend = true;
        if (isSameBackendNode(targetBackendBaseUrl, gatewayBaseUrl)) {
          markBackendUnavailable(gatewayBaseUrl);
          gatewayTransportFailed = true;
          break;
        }

        markBackendUnavailable(targetBackendBaseUrl);
        continue;
      }

      const routeInfo = resolveBackendRouteInfo(response, targetBackendBaseUrl, gatewayBaseUrl);
      markBackendHealthy(routeInfo.gatewayBaseUrl);
      markBackendHealthy(routeInfo.backendBaseUrl);
      currentGatewayBaseUrl = routeInfo.gatewayBaseUrl;

      if (path === BACKEND_HEALTH_CHECK_PATH && response.ok && isServerHealthData(payload.data)) {
        registerBackendPool([routeInfo.backendBaseUrl, routeInfo.gatewayBaseUrl, payload.data.nodeBaseUrl ?? ""]);
        registerBackendPool(payload.data.proxyBackendUrls ?? []);
        updateBackendLoad(routeInfo.backendBaseUrl, payload.data);
      }

      onBackendResolved?.(routeInfo);

      if (!response.ok) {
        const message = payload.error?.message ?? `Request failed (${response.status})`;
        throw new Error(message);
      }
      if (typeof payload.data === "undefined") {
        throw new Error("Malformed API response");
      }

      return payload.data;
    }

    if (gatewayTransportFailed) {
      continue;
    }
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

function buildBackendAttemptOrder(preferredBackendBaseUrl?: string, preferLeastLoaded = false): string[] {
  let roundRobinOrder = buildRoundRobinOrder();
  if (preferLeastLoaded) {
    roundRobinOrder = sortBackendsByLoad(roundRobinOrder);
  }

  const preferredBackend = preferredBackendBaseUrl ? parseApiBaseUrl(preferredBackendBaseUrl) : null;
  if (!preferredBackend) {
    return roundRobinOrder;
  }

  ensureBackendInPool(preferredBackend);
  const resolvedPreferredBackend = findBackendInPool(preferredBackend) ?? preferredBackend;

  if (!roundRobinOrder.some((backendBaseUrl) => isSameBackendNode(backendBaseUrl, resolvedPreferredBackend))) {
    roundRobinOrder = [resolvedPreferredBackend, ...roundRobinOrder];
  }

  return [
    resolvedPreferredBackend,
    ...roundRobinOrder.filter((backendBaseUrl) => !isSameBackendNode(backendBaseUrl, resolvedPreferredBackend)),
  ];
}

function sortBackendsByLoad(baseUrls: string[]): string[] {
  const withPriority = baseUrls.map((baseUrl, index) => ({
    baseUrl,
    index,
    score: getBackendLoadScore(baseUrl),
  }));

  withPriority.sort((left, right) => {
    if (left.score === right.score) {
      return left.index - right.index;
    }
    return left.score - right.score;
  });

  return withPriority.map((item) => item.baseUrl);
}

function buildGatewayAttemptOrder(): string[] {
  const gatewayBaseUrl = resolveApiBaseUrl(currentGatewayBaseUrl);
  const fallbackOrder = getBackendPoolSnapshot().filter((backendBaseUrl) => !isSameBackendNode(backendBaseUrl, gatewayBaseUrl));
  return [gatewayBaseUrl, ...fallbackOrder];
}

function buildRoundRobinOrder(): string[] {
  const knownBackends = getBackendPoolSnapshot();
  const startIndex = nextApiBaseUrlIndex % knownBackends.length;
  nextApiBaseUrlIndex = (startIndex + 1) % knownBackends.length;

  const ordered: string[] = [];
  for (let offset = 0; offset < knownBackends.length; offset += 1) {
    ordered.push(knownBackends[(startIndex + offset) % knownBackends.length]);
  }
  return ordered;
}

function getBackendPoolSnapshot(): string[] {
  if (backendPool.length === 0) {
    backendPool.push(DEFAULT_API_BASE_URL);
  }

  return [...backendPool];
}

function ensureBackendInPool(backendBaseUrl: string): boolean {
  const existingBackend = findBackendInPool(backendBaseUrl);
  if (existingBackend) {
    const preferredBackend = selectPreferredBackendURL(existingBackend, backendBaseUrl);
    if (preferredBackend === existingBackend) {
      return false;
    }

    const backendIndex = backendPool.findIndex((value) => value === existingBackend);
    if (backendIndex === -1) {
      return false;
    }

    backendPool[backendIndex] = preferredBackend;
    migrateBackendState(existingBackend, preferredBackend);
    if (isSameBackendNode(currentGatewayBaseUrl, existingBackend)) {
      currentGatewayBaseUrl = preferredBackend;
    }

    return true;
  }

  backendPool.push(backendBaseUrl);
  return true;
}

function findBackendInPool(backendBaseUrl: string): string | null {
  const targetNodeKey = backendNodeKey(backendBaseUrl);
  if (!targetNodeKey) {
    return null;
  }

  for (const existingBackend of backendPool) {
    if (backendNodeKey(existingBackend) === targetNodeKey) {
      return existingBackend;
    }
  }

  return null;
}

function selectPreferredBackendURL(currentBackendBaseUrl: string, candidateBackendBaseUrl: string): string {
  if (currentBackendBaseUrl === candidateBackendBaseUrl) {
    return currentBackendBaseUrl;
  }

  try {
    const current = new URL(currentBackendBaseUrl);
    const candidate = new URL(candidateBackendBaseUrl);

    if (candidate.protocol === "https:" && current.protocol !== "https:") {
      return candidateBackendBaseUrl;
    }
  } catch {
    return currentBackendBaseUrl;
  }

  return currentBackendBaseUrl;
}

function migrateBackendState(fromBackendBaseUrl: string, toBackendBaseUrl: string): void {
  if (fromBackendBaseUrl === toBackendBaseUrl) {
    return;
  }

  const fromHealthState = backendHealthStates.get(fromBackendBaseUrl);
  const toHealthState = backendHealthStates.get(toBackendBaseUrl);
  if (fromHealthState && !toHealthState) {
    backendHealthStates.set(toBackendBaseUrl, fromHealthState);
  } else if (fromHealthState && toHealthState && fromHealthState.checkedAtMs > toHealthState.checkedAtMs) {
    backendHealthStates.set(toBackendBaseUrl, fromHealthState);
  }
  backendHealthStates.delete(fromBackendBaseUrl);

  const fromLoadState = backendLoadStates.get(fromBackendBaseUrl);
  const toLoadState = backendLoadStates.get(toBackendBaseUrl);
  if (fromLoadState && !toLoadState) {
    backendLoadStates.set(toBackendBaseUrl, fromLoadState);
  } else if (fromLoadState && toLoadState && fromLoadState.checkedAtMs > toLoadState.checkedAtMs) {
    backendLoadStates.set(toBackendBaseUrl, fromLoadState);
  }
  backendLoadStates.delete(fromBackendBaseUrl);
}

function isSameBackendNode(leftBackendBaseUrl: string, rightBackendBaseUrl: string): boolean {
  const leftKey = backendNodeKey(leftBackendBaseUrl);
  const rightKey = backendNodeKey(rightBackendBaseUrl);
  return leftKey !== null && rightKey !== null && leftKey === rightKey;
}

function backendNodeKey(backendBaseUrl: string): string | null {
  try {
    const parsed = new URL(backendBaseUrl);
    const host = parsed.hostname.toLowerCase();
    const port = parsed.port || (parsed.protocol === "https:" ? "443" : "80");
    if (!host) {
      return null;
    }

    return `${host}:${port}`;
  } catch {
    return null;
  }
}

function normalizeHttpMethod(method?: string): string {
  return (method ?? "GET").toUpperCase();
}

function shouldPreferLeastLoaded(path: string, method: string, preferredBackendBaseUrl?: string): boolean {
  if (preferredBackendBaseUrl) {
    return false;
  }

  return path === "/api/jobs" && method === "POST";
}

function isServerHealthData(value: unknown): value is ServerHealth {
  if (!value || typeof value !== "object") {
    return false;
  }

  const candidate = value as Partial<ServerHealth>;
  return (
    typeof candidate.status === "string" &&
    typeof candidate.captchaRequired === "boolean" &&
    typeof candidate.runningBuilds === "number" &&
    typeof candidate.queuedBuilds === "number" &&
    typeof candidate.concurrentBuilds === "number"
  );
}

function updateBackendLoad(backendBaseUrl: string, health: ServerHealth): void {
  backendLoadStates.set(backendBaseUrl, {
    runningBuilds: Math.max(0, Math.floor(health.runningBuilds)),
    queuedBuilds: Math.max(0, Math.floor(health.queuedBuilds)),
    concurrentBuilds: Math.max(1, Math.floor(health.concurrentBuilds)),
    checkedAtMs: Date.now(),
  });
}

function getBackendLoadScore(backendBaseUrl: string): number {
  const loadState = backendLoadStates.get(backendBaseUrl);
  if (!loadState) {
    return Number.MAX_SAFE_INTEGER - 1;
  }

  const ageMs = Date.now() - loadState.checkedAtMs;
  if (ageMs > BACKEND_LOAD_INFO_TTL_MS) {
    return Number.MAX_SAFE_INTEGER;
  }

  const capacity = Math.max(1, loadState.concurrentBuilds);
  return (loadState.runningBuilds + loadState.queuedBuilds) / capacity;
}

async function isGatewayAvailable(gatewayBaseUrl: string): Promise<boolean> {
  const backendState = getBackendHealthState(gatewayBaseUrl);
  const nowMs = Date.now();

  if (backendState.unavailableUntilMs > nowMs) {
    return false;
  }

  if (backendState.checkedAtMs > 0 && nowMs - backendState.checkedAtMs < BACKEND_HEALTH_CACHE_TTL_MS) {
    return backendState.healthy;
  }

  const healthy = await probeGatewayHealth(gatewayBaseUrl);
  if (healthy) {
    markBackendHealthy(gatewayBaseUrl, nowMs);
    return true;
  }

  markBackendUnavailable(gatewayBaseUrl, nowMs);
  return false;
}

function isBackendInCooldown(backendBaseUrl: string): boolean {
  const backendState = getBackendHealthState(backendBaseUrl);
  return backendState.unavailableUntilMs > Date.now();
}

function getBackendHealthState(backendBaseUrl: string): BackendHealthState {
  ensureBackendInPool(backendBaseUrl);
  const resolvedBackendBaseUrl = findBackendInPool(backendBaseUrl) ?? backendBaseUrl;

  const existingState = backendHealthStates.get(resolvedBackendBaseUrl);
  if (existingState) {
    return existingState;
  }

  const fallbackState: BackendHealthState = {
    healthy: true,
    checkedAtMs: 0,
    unavailableUntilMs: 0,
  };
  backendHealthStates.set(resolvedBackendBaseUrl, fallbackState);
  return fallbackState;
}

function markBackendHealthy(backendBaseUrl: string, nowMs = Date.now()): void {
  const backendState = getBackendHealthState(backendBaseUrl);
  const changed = !backendState.healthy || backendState.unavailableUntilMs !== 0;
  backendState.healthy = true;
  backendState.checkedAtMs = nowMs;
  backendState.unavailableUntilMs = 0;

  if (changed) {
    emitBackendHealthChanged();
  }
}

function markBackendUnavailable(backendBaseUrl: string, nowMs = Date.now()): void {
  const backendState = getBackendHealthState(backendBaseUrl);
  const previousUnavailableUntilMs = backendState.unavailableUntilMs;
  const nextUnavailableUntilMs = nowMs + BACKEND_UNAVAILABLE_COOLDOWN_MS;
  const changed = backendState.healthy || previousUnavailableUntilMs !== nextUnavailableUntilMs;

  backendState.healthy = false;
  backendState.checkedAtMs = nowMs;
  backendState.unavailableUntilMs = nextUnavailableUntilMs;

  if (changed) {
    emitBackendHealthChanged();
  }
}

async function probeGatewayHealth(gatewayBaseUrl: string): Promise<boolean> {
  const controller = new AbortController();
  const timeoutId = globalThis.setTimeout(() => {
    controller.abort();
  }, BACKEND_HEALTH_TIMEOUT_MS);

  try {
    const response = await fetch(apiUrl(BACKEND_HEALTH_CHECK_PATH, gatewayBaseUrl), {
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

function buildRequestHeaders(
  sourceHeaders: HeadersInit | undefined,
  targetBackendBaseUrl: string,
  gatewayBaseUrl: string,
): Headers {
  const headers = new Headers(sourceHeaders ?? {});
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  if (!isSameBackendNode(targetBackendBaseUrl, gatewayBaseUrl)) {
    headers.set(TARGET_BACKEND_HEADER, targetBackendBaseUrl);
  } else {
    headers.delete(TARGET_BACKEND_HEADER);
  }

  return headers;
}

function resolveBackendRouteInfo(response: Response, targetBackendBaseUrl: string, gatewayBaseUrl: string): BackendRouteInfo {
  const servedBy = parseApiBaseUrl(response.headers.get(RESPONSE_SERVED_BY_HEADER) ?? "") ?? targetBackendBaseUrl;
  const proxiedVia = parseApiBaseUrl(response.headers.get(RESPONSE_PROXIED_VIA_HEADER) ?? "") ?? gatewayBaseUrl;

  return {
    backendBaseUrl: servedBy,
    gatewayBaseUrl: proxiedVia,
    proxied: !isSameBackendNode(servedBy, proxiedVia),
  };
}

function emitBackendHealthChanged(): void {
  for (const listener of backendHealthListeners) {
    listener();
  }
}

function throwIfAborted(signal?: AbortSignal | null): void {
  if (signal?.aborted) {
    throw new DOMException("Request aborted", "AbortError");
  }
}

function isAbortError(value: unknown): value is DOMException {
  return value instanceof DOMException && value.name === "AbortError";
}

function appendTargetBackendQuery(urlValue: string, targetBackendBaseUrl: string): string {
  const url = new URL(urlValue);
  url.searchParams.set(TARGET_BACKEND_QUERY_PARAM, targetBackendBaseUrl);
  return url.toString();
}

async function parseApiPayload<T>(response: Response): Promise<{ data?: T; error?: { code?: string; message?: string } }> {
  try {
    return (await response.json()) as { data?: T; error?: { code?: string; message?: string } };
  } catch {
    return {};
  }
}

function resolveApiBaseUrl(backendBaseUrl?: string): string {
  const knownBackends = getBackendPoolSnapshot();
  if (!backendBaseUrl) {
    return knownBackends[0];
  }

  const parsedBaseUrl = parseApiBaseUrl(backendBaseUrl);
  if (!parsedBaseUrl) {
    return knownBackends[0];
  }

  ensureBackendInPool(parsedBaseUrl);

  return findBackendInPool(parsedBaseUrl) ?? parsedBaseUrl;
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
