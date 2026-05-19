import { afterEach, describe, expect, it, vi } from "vitest";

describe("apiUrl", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("joins configured API base with path", async () => {
    vi.stubEnv("VITE_API_BASE_URL", "http://api.example.com");
    const { apiUrl } = await import("./api");
    expect(apiUrl("/api/healthz")).toBe("http://api.example.com/api/healthz");
  });

  it("strips trailing slash from configured base URL", async () => {
    vi.stubEnv("VITE_API_BASE_URL", "http://api.example.com/");
    const { apiUrl } = await import("./api");
    expect(apiUrl("/api/captcha")).toBe("http://api.example.com/api/captcha");
  });
});
