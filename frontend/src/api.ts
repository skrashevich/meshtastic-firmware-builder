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
  captchaId: string;
  question: string;
  expiresAt: string;
}

export interface LogsSnapshot {
  lines: string[];
}

const API_BASE_URL = stripTrailingSlash(import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080");

export function apiUrl(path: string): string {
  return `${API_BASE_URL}${path}`;
}

export async function discoverDevices(repoUrl: string, ref: string): Promise<DiscoverResponse> {
  return request<DiscoverResponse>("/api/repos/discover", {
    method: "POST",
    body: JSON.stringify({ repoUrl, ref }),
  });
}

export async function discoverRepoRefs(repoUrl: string, signal?: AbortSignal): Promise<RepoRefsResponse> {
  return request<RepoRefsResponse>("/api/repos/refs", {
    method: "POST",
    body: JSON.stringify({ repoUrl }),
    signal,
  });
}

export async function createBuildJob(
  repoUrl: string,
  ref: string,
  device: string,
  captchaId: string,
  captchaAnswer: string,
): Promise<JobState> {
  return request<JobState>("/api/jobs", {
    method: "POST",
    body: JSON.stringify({ repoUrl, ref, device, captchaId, captchaAnswer }),
  });
}

export async function getCaptchaChallenge(): Promise<CaptchaChallenge> {
  return request<CaptchaChallenge>("/api/captcha", {
    method: "GET",
  });
}

export async function getJob(jobId: string): Promise<JobState> {
  return request<JobState>(`/api/jobs/${jobId}`);
}

export async function getLogs(jobId: string): Promise<LogsSnapshot> {
  return request<LogsSnapshot>(`/api/jobs/${jobId}/logs`);
}

export async function getArtifacts(jobId: string): Promise<ArtifactItem[]> {
  const response = await request<{ artifacts: ArtifactItem[] }>(`/api/jobs/${jobId}/artifacts`);
  return response.artifacts;
}

export function createLogStream(jobId: string): EventSource {
  return new EventSource(apiUrl(`/api/jobs/${jobId}/logs/stream`));
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiUrl(path), {
    headers: {
      "Content-Type": "application/json",
    },
    ...init,
  });

  const payload = (await response.json()) as { data?: T; error?: { message?: string } };
  if (!response.ok) {
    const message = payload.error?.message ?? "Request failed";
    throw new Error(message);
  }
  if (!payload.data) {
    throw new Error("Malformed API response");
  }

  return payload.data;
}

function stripTrailingSlash(value: string): string {
  if (value.endsWith("/")) {
    return value.slice(0, -1);
  }
  return value;
}
