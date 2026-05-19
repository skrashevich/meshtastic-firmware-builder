import { describe, expect, it } from "vitest";
import type { RepoRefsResponse } from "./api";
import {
  collectRefSuggestions,
  formatQueueETA,
  formatSize,
  limitRefItems,
  looksLikeRepoURL,
  parseMultilineValues,
  readInitialFormValues,
  syncFormValuesToURL,
} from "./appUtils";

describe("formatSize", () => {
  it("formats bytes, kilobytes, and megabytes", () => {
    expect(formatSize(512)).toBe("512 B");
    expect(formatSize(2048)).toBe("2.0 KB");
    expect(formatSize(5 * 1024 * 1024)).toBe("5.0 MB");
  });
});

describe("formatQueueETA", () => {
  it("formats Russian queue ETA", () => {
    expect(formatQueueETA(90, "ru")).toBe("2 мин");
    expect(formatQueueETA(3700, "ru")).toBe("1 ч 2 мин");
  });

  it("formats English queue ETA", () => {
    expect(formatQueueETA(90, "en")).toBe("2m");
    expect(formatQueueETA(3700, "en")).toBe("1h 2m");
  });
});

describe("parseMultilineValues", () => {
  it("trims and drops empty lines", () => {
    expect(parseMultilineValues("  -DTEST=1 \n\n -Wall \n")).toEqual(["-DTEST=1", "-Wall"]);
  });
});

describe("collectRefSuggestions", () => {
  it("deduplicates default branch, branches, and tags", () => {
    const repoRefs: RepoRefsResponse = {
      repoUrl: "https://github.com/example/repo",
      defaultBranch: "main",
      recentBranches: [{ name: "main" }, { name: "develop" }],
      recentTags: [{ name: "v1.0.0" }, { name: "develop" }],
    };

    expect(collectRefSuggestions(repoRefs)).toEqual(["main", "develop", "v1.0.0"]);
  });
});

describe("limitRefItems", () => {
  it("returns original list when limit is not smaller", () => {
    const items = [{ name: "main" }, { name: "develop" }];
    expect(limitRefItems(items, 5)).toBe(items);
  });

  it("truncates list when limit is smaller", () => {
    const items = [{ name: "main" }, { name: "develop" }, { name: "feature" }];
    expect(limitRefItems(items, 2)).toEqual([{ name: "main" }, { name: "develop" }]);
  });
});

describe("looksLikeRepoURL", () => {
  it("accepts common repository URL formats", () => {
    expect(looksLikeRepoURL("https://github.com/meshtastic/firmware")).toBe(true);
    expect(looksLikeRepoURL("git@github.com:meshtastic/firmware.git")).toBe(true);
    expect(looksLikeRepoURL("not-a-url")).toBe(false);
  });
});

describe("readInitialFormValues", () => {
  it("reads repo and ref from query string", () => {
    const search = "?repo=https%3A%2F%2Fgithub.com%2Fowner%2Frepo&ref=main";
    expect(readInitialFormValues("https://github.com/default/repo", search)).toEqual({
      repoUrl: "https://github.com/owner/repo",
      ref: "main",
      hasRefInQuery: true,
    });
  });

  it("falls back to default repository", () => {
    expect(readInitialFormValues("https://github.com/default/repo", "")).toEqual({
      repoUrl: "https://github.com/default/repo",
      ref: "",
      hasRefInQuery: false,
    });
  });
});

describe("syncFormValuesToURL", () => {
  it("updates repo and ref query params", () => {
    const updates: string[] = [];
    syncFormValuesToURL(
      "https://github.com/owner/repo",
      "main",
      "http://localhost:5173/?repo=old&ref=dev#build",
      (nextURL) => updates.push(nextURL),
    );

    expect(updates).toEqual([
      "/?repo=https%3A%2F%2Fgithub.com%2Fowner%2Frepo&ref=main#build",
    ]);
  });

  it("skips history update when query string is unchanged", () => {
    const updates: string[] = [];
    syncFormValuesToURL(
      "https://github.com/owner/repo",
      "main",
      "http://localhost:5173/?repo=https%3A%2F%2Fgithub.com%2Fowner%2Frepo&ref=main",
      (nextURL) => updates.push(nextURL),
    );

    expect(updates).toEqual([]);
  });
});
