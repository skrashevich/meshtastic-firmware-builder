import type { RepoRefsResponse } from "./api";
import type { Locale } from "./i18n";

export type InitialFormValues = {
  repoUrl: string;
  ref: string;
  hasRefInQuery: boolean;
};

export function errorToMessage(value: unknown, fallback: string): string {
  if (value instanceof Error) {
    return value.message;
  }
  return fallback;
}

export function formatSize(size: number): string {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatQueueETA(seconds: number, locale: Locale): string {
  const totalMinutes = Math.max(1, Math.ceil(seconds / 60));
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;

  if (locale === "ru") {
    if (hours > 0 && minutes > 0) {
      return `${hours} ч ${minutes} мин`;
    }
    if (hours > 0) {
      return `${hours} ч`;
    }
    return `${totalMinutes} мин`;
  }

  if (hours > 0 && minutes > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (hours > 0) {
    return `${hours}h`;
  }
  return `${totalMinutes}m`;
}

export function parseMultilineValues(raw: string): string[] {
  return raw
    .split("\n")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

export function collectRefSuggestions(repoRefs: RepoRefsResponse | null): string[] {
  if (!repoRefs) {
    return [];
  }

  const seen = new Set<string>();
  const result: string[] = [];

  if (repoRefs.defaultBranch) {
    const branch = repoRefs.defaultBranch.trim();
    if (branch && !seen.has(branch)) {
      seen.add(branch);
      result.push(branch);
    }
  }

  for (const item of repoRefs.recentBranches) {
    if (!item.name || seen.has(item.name)) {
      continue;
    }
    seen.add(item.name);
    result.push(item.name);
  }

  for (const item of repoRefs.recentTags) {
    if (!item.name || seen.has(item.name)) {
      continue;
    }
    seen.add(item.name);
    result.push(item.name);
  }

  return result;
}

export function limitRefItems<T extends { name: string }>(items: T[], limit: number): T[] {
  if (limit < 1 || items.length <= limit) {
    return items;
  }
  return items.slice(0, limit);
}

export function looksLikeRepoURL(value: string): boolean {
  return /^(https?:\/\/|ssh:\/\/|git:\/\/|git@)/.test(value);
}

export function readInitialFormValues(
  defaultRepo: string,
  search = "",
): InitialFormValues {
  const params = new URLSearchParams(search);
  const initialRepo = (params.get("repo") ?? params.get("repoUrl") ?? "").trim();
  const initialRef = (params.get("ref") ?? "").trim();

  return {
    repoUrl: initialRepo || defaultRepo,
    ref: initialRef,
    hasRefInQuery: initialRef.length > 0,
  };
}

export function syncFormValuesToURL(
  repoUrl: string,
  ref: string,
  currentHref: string,
  replaceState: (url: string) => void,
): void {
  const url = new URL(currentHref);
  const params = new URLSearchParams(url.search);
  params.delete("repoUrl");

  setQueryParam(params, "repo", repoUrl.trim());
  setQueryParam(params, "ref", ref.trim());

  const currentSearch = url.searchParams.toString();
  const nextSearch = params.toString();
  if (currentSearch === nextSearch) {
    return;
  }

  const searchPart = nextSearch ? `?${nextSearch}` : "";
  const nextURL = `${url.pathname}${searchPart}${url.hash}`;
  replaceState(nextURL);
}

export function setQueryParam(params: URLSearchParams, key: string, value: string): void {
  if (!value) {
    params.delete(key);
    return;
  }
  params.set(key, value);
}
