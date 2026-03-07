import { FormEvent, useState } from "react";
import { BuildLog, BuildLogEntry, StatsSummary, getBuildLog, getBuildLogs, getStats } from "./api";
import { Locale, dict } from "./i18n";

const RECENT_STEPS = [50, 150, 500];
const TOP_STEPS = [10, 30, 100];

export default function StatsPage() {
  const [locale, setLocale] = useState<Locale>("ru");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [data, setData] = useState<StatsSummary | null>(null);
  const [recentLimit, setRecentLimit] = useState(RECENT_STEPS[0]);
  const [topLimit, setTopLimit] = useState(TOP_STEPS[0]);
  const [buildLogs, setBuildLogs] = useState<BuildLogEntry[]>([]);
  const [buildLogsLoading, setBuildLogsLoading] = useState(false);
  const [selectedLog, setSelectedLog] = useState<BuildLog | null>(null);
  const [selectedLogLoading, setSelectedLogLoading] = useState(false);
  const [selectedLogError, setSelectedLogError] = useState("");

  const t = dict[locale];

  async function fetchStats(opts?: { recent?: number; top?: number }) {
    const rl = opts?.recent ?? recentLimit;
    const tl = opts?.top ?? topLimit;
    setLoading(true);
    setError("");
    try {
      const [summary, logs] = await Promise.all([
        getStats(password, { recentLimit: rl, topLimit: tl }),
        getBuildLogs(password, 100),
      ]);
      setData(summary);
      setBuildLogs(logs);
      setRecentLimit(rl);
      setTopLimit(tl);
    } catch (err) {
      setError(err instanceof Error ? err.message : t.statsRequestError);
    } finally {
      setLoading(false);
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    await fetchStats();
  }

  const eventLabelsI18n: Record<string, string> = {
    visit: t.statsEventVisit,
    discover: t.statsEventDiscover,
    build: t.statsEventBuild,
    download: t.statsEventDownload,
  };

  return (
    <div className="page-shell">
      <div className="layout" style={{ maxWidth: 960, margin: "0 auto", padding: "28px 16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 8 }}>
          <a href="/" style={{ color: "var(--accent)", textDecoration: "none", fontSize: 14 }}>
            &larr; {t.statsBack}
          </a>
          <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, flex: 1 }}>{t.statsTitle}</h1>
          <div style={{ display: "flex", gap: 4 }}>
            <button
              className={locale === "ru" ? "locale-btn active" : "locale-btn"}
              onClick={() => setLocale("ru")}
            >
              RU
            </button>
            <button
              className={locale === "en" ? "locale-btn active" : "locale-btn"}
              onClick={() => setLocale("en")}
            >
              EN
            </button>
          </div>
        </div>

        {!data && (
          <div
            style={{
              background: "var(--surface)",
              borderRadius: "var(--radius)",
              padding: 24,
              boxShadow: "var(--shadow)",
              maxWidth: 400,
            }}
          >
            <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <label style={{ fontWeight: 600, fontSize: 14 }}>{t.statsPasswordLabel}</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t.statsPasswordPlaceholder}
                autoFocus
                style={{
                  padding: "10px 14px",
                  border: "1.5px solid var(--line)",
                  borderRadius: 10,
                  fontSize: 14,
                  fontFamily: "inherit",
                  outline: "none",
                }}
              />
              {error && <p style={{ color: "var(--danger)", margin: 0, fontSize: 13 }}>{error}</p>}
              <button
                type="submit"
                disabled={loading || !password}
                style={{
                  background: "var(--accent)",
                  color: "#fff",
                  border: "none",
                  borderRadius: 10,
                  padding: "10px 20px",
                  fontSize: 14,
                  fontWeight: 600,
                  cursor: loading || !password ? "not-allowed" : "pointer",
                  opacity: loading || !password ? 0.6 : 1,
                }}
              >
                {loading ? t.statsLoading : t.statsShow}
              </button>
            </form>
          </div>
        )}

        {data && (
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {/* Counters */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: 12 }}>
              {[
                { label: t.statsVisits, value: data.totalVisits, color: "#0f8f7f" },
                { label: t.statsDiscovers, value: data.totalDiscovers, color: "#5c7cfa" },
                { label: t.statsBuilds, value: data.totalBuilds, color: "#f76707" },
                { label: t.statsDownloads, value: data.totalDownloads, color: "#2f9e44" },
                { label: t.statsUniqueIPs, value: data.uniqueIPs, color: "#ae3ec9" },
              ].map((item) => (
                <div
                  key={item.label}
                  style={{
                    background: "var(--surface)",
                    borderRadius: "var(--radius)",
                    padding: "16px 20px",
                    boxShadow: "var(--shadow)",
                    borderLeft: `4px solid ${item.color}`,
                  }}
                >
                  <div style={{ fontSize: 28, fontWeight: 700, color: item.color }}>{item.value}</div>
                  <div style={{ fontSize: 12, color: "var(--ink-muted)", marginTop: 2 }}>{item.label}</div>
                </div>
              ))}
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
              {/* Top repos */}
              {data.topRepos.length > 0 && (
                <StatsTable
                  title={`${t.statsTopRepos} (${data.topRepos.length})`}
                  headers={[t.statsRepoHeader, t.statsRequestsHeader]}
                  rows={data.topRepos.map((r) => [shortenUrl(r.name), String(r.count)])}
                />
              )}

              {/* Top devices */}
              {data.topDevices.length > 0 && (
                <StatsTable
                  title={`${t.statsTopDevices} (${data.topDevices.length})`}
                  headers={[t.statsDeviceHeader, t.statsBuildsHeader]}
                  rows={data.topDevices.map((d) => [d.name, String(d.count)])}
                />
              )}
            </div>

            {(data.topRepos.length > 0 || data.topDevices.length > 0) && (
              <LimitSelector
                label={t.statsTopLabel}
                steps={TOP_STEPS}
                current={topLimit}
                loading={loading}
                onChange={(v) => fetchStats({ top: v })}
              />
            )}

            {/* Daily summary */}
            {data.dailySummary.length > 0 && (
              <div
                style={{
                  background: "var(--surface)",
                  borderRadius: "var(--radius)",
                  padding: 20,
                  boxShadow: "var(--shadow)",
                }}
              >
                <h3 style={{ margin: "0 0 12px", fontSize: 15, fontWeight: 600 }}>{t.statsByDay}</h3>
                <div style={{ overflowX: "auto" }}>
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                    <thead>
                      <tr style={{ borderBottom: "1.5px solid var(--line)" }}>
                        {[t.statsDayDate, t.statsDayVisits, t.statsDayDiscovers, t.statsDayBuilds, t.statsDayDownloads].map((h, i) => (
                          <th
                            key={h}
                            style={{
                              padding: "6px 10px",
                              textAlign: i === 0 ? "left" : "right",
                              color: "var(--ink-muted)",
                              fontWeight: 600,
                            }}
                          >
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {[...data.dailySummary].reverse().map((day) => (
                        <tr key={day.date} style={{ borderBottom: "1px solid var(--line)" }}>
                          <td style={{ padding: "6px 10px", fontFamily: "IBM Plex Mono, monospace" }}>{day.date}</td>
                          <td style={{ padding: "6px 10px", textAlign: "right" }}>{day.visits}</td>
                          <td style={{ padding: "6px 10px", textAlign: "right" }}>{day.discovers}</td>
                          <td style={{ padding: "6px 10px", textAlign: "right" }}>{day.builds}</td>
                          <td style={{ padding: "6px 10px", textAlign: "right" }}>{day.downloads}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {/* Firmware cache */}
            {data.firmwareCache && (
              <div
                style={{
                  background: "var(--surface)",
                  borderRadius: "var(--radius)",
                  padding: 20,
                  boxShadow: "var(--shadow)",
                }}
              >
                <h3 style={{ margin: "0 0 12px", fontSize: 15, fontWeight: 600 }}>
                  {t.statsCacheTitle} — {data.firmwareCache.entryCount} {t.statsCacheEntries}
                  {data.firmwareCache.totalSize > 0 && (
                    <span style={{ fontWeight: 400, color: "var(--ink-muted)", marginLeft: 8 }}>
                      ({t.statsCacheTotalSize}: {formatBytes(data.firmwareCache.totalSize)})
                    </span>
                  )}
                </h3>
                {data.firmwareCache.entries.length === 0 ? (
                  <p style={{ color: "var(--ink-muted)", margin: 0, fontSize: 13 }}>{t.statsCacheEmpty}</p>
                ) : (
                  <div style={{ overflowX: "auto" }}>
                    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
                      <thead>
                        <tr style={{ borderBottom: "1.5px solid var(--line)" }}>
                          {[t.statsCacheKey, t.statsCacheCreated, t.statsCacheArtifacts, t.statsCacheSize].map((h) => (
                            <th
                              key={h}
                              style={{
                                padding: "6px 8px",
                                textAlign: "left",
                                color: "var(--ink-muted)",
                                fontWeight: 600,
                                whiteSpace: "nowrap",
                              }}
                            >
                              {h}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {data.firmwareCache.entries.map((entry) => (
                          <tr key={entry.key} style={{ borderBottom: "1px solid var(--line)" }}>
                            <td
                              style={{
                                padding: "5px 8px",
                                fontFamily: "IBM Plex Mono, monospace",
                                fontSize: 11,
                              }}
                              title={entry.key}
                            >
                              {entry.key.slice(0, 12)}…
                            </td>
                            <td style={{ padding: "5px 8px", whiteSpace: "nowrap" }}>
                              {formatTs(entry.createdAt, locale)}
                            </td>
                            <td style={{ padding: "5px 8px" }}>
                              {entry.artifacts.map((a) => a.name).join(", ")}
                            </td>
                            <td style={{ padding: "5px 8px", whiteSpace: "nowrap", textAlign: "right" }}>
                              {formatBytes(entry.totalSize)}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}

            {/* Build logs */}
            <div
              style={{
                background: "var(--surface)",
                borderRadius: "var(--radius)",
                padding: 20,
                boxShadow: "var(--shadow)",
              }}
            >
              <h3 style={{ margin: "0 0 12px", fontSize: 15, fontWeight: 600 }}>
                {t.statsBuildLogsTitle} ({buildLogs.length})
              </h3>
              {buildLogs.length === 0 ? (
                <p style={{ color: "var(--ink-muted)", margin: 0, fontSize: 13 }}>{t.statsBuildLogsEmpty}</p>
              ) : (
                <div style={{ overflowX: "auto" }}>
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
                    <thead>
                      <tr style={{ borderBottom: "1.5px solid var(--line)" }}>
                        {[t.statsBuildLogsJob, t.statsBuildLogsRepo, t.statsBuildLogsDevice, t.statsBuildLogsStatus, t.statsBuildLogsCreated, t.statsBuildLogsDuration, t.statsBuildLogsLines, "\u00a0"].map((h) => (
                          <th
                            key={h}
                            style={{
                              padding: "6px 8px",
                              textAlign: "left",
                              color: "var(--ink-muted)",
                              fontWeight: 600,
                              whiteSpace: "nowrap",
                            }}
                          >
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {buildLogs.map((bl) => (
                        <tr key={bl.jobId} style={{ borderBottom: "1px solid var(--line)" }}>
                          <td style={{ padding: "5px 8px", fontFamily: "IBM Plex Mono, monospace", fontSize: 11 }} title={bl.jobId}>
                            {bl.jobId.slice(0, 8)}…
                          </td>
                          <td
                            style={{
                              padding: "5px 8px",
                              maxWidth: 180,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                            title={bl.repoUrl}
                          >
                            {bl.repoUrl ? shortenUrl(bl.repoUrl) : "\u2014"}
                          </td>
                          <td style={{ padding: "5px 8px" }}>{bl.device}</td>
                          <td style={{ padding: "5px 8px" }}>
                            <BuildStatusBadge status={bl.status} />
                          </td>
                          <td style={{ padding: "5px 8px", whiteSpace: "nowrap" }}>
                            {formatTs(bl.createdAt, locale)}
                          </td>
                          <td style={{ padding: "5px 8px", whiteSpace: "nowrap", fontFamily: "IBM Plex Mono, monospace" }}>
                            {bl.startedAt && bl.finishedAt ? formatDuration(bl.startedAt, bl.finishedAt) : "\u2014"}
                          </td>
                          <td style={{ padding: "5px 8px", textAlign: "right" }}>{bl.lineCount}</td>
                          <td style={{ padding: "5px 8px" }}>
                            <button
                              onClick={async () => {
                                setSelectedLog(null);
                                setSelectedLogError("");
                                setSelectedLogLoading(true);
                                try {
                                  const log = await getBuildLog(password, bl.jobId);
                                  setSelectedLog(log);
                                } catch (err) {
                                  setSelectedLogError(err instanceof Error ? err.message : t.statsBuildLogsLoadError);
                                } finally {
                                  setSelectedLogLoading(false);
                                }
                              }}
                              disabled={selectedLogLoading}
                              style={{
                                background: "none",
                                border: "1px solid var(--accent)",
                                color: "var(--accent)",
                                borderRadius: 6,
                                padding: "2px 8px",
                                fontSize: 11,
                                cursor: selectedLogLoading ? "not-allowed" : "pointer",
                                whiteSpace: "nowrap",
                              }}
                            >
                              {t.statsBuildLogsView}
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* Build log viewer modal */}
            {(selectedLog || selectedLogLoading || selectedLogError) && (
              <div
                style={{
                  position: "fixed",
                  inset: 0,
                  background: "rgba(0,0,0,0.6)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  zIndex: 1000,
                  padding: 20,
                }}
                onClick={() => {
                  setSelectedLog(null);
                  setSelectedLogError("");
                }}
              >
                <div
                  style={{
                    background: "var(--surface)",
                    borderRadius: "var(--radius)",
                    padding: 24,
                    width: "100%",
                    maxWidth: 900,
                    maxHeight: "85vh",
                    display: "flex",
                    flexDirection: "column",
                    boxShadow: "0 8px 32px rgba(0,0,0,0.3)",
                  }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {selectedLogLoading && (
                    <p style={{ color: "var(--ink-muted)", margin: 0 }}>{t.statsBuildLogsLoading}</p>
                  )}
                  {selectedLogError && (
                    <p style={{ color: "var(--danger)", margin: 0 }}>{selectedLogError}</p>
                  )}
                  {selectedLog && (
                    <>
                      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 12 }}>
                        <div>
                          <h3 style={{ margin: "0 0 4px", fontSize: 15, fontWeight: 600 }}>
                            {t.statsBuildLogsJob}: {selectedLog.jobId.slice(0, 12)}…
                          </h3>
                          <div style={{ fontSize: 12, color: "var(--ink-muted)", display: "flex", gap: 12, flexWrap: "wrap" }}>
                            <span>{selectedLog.repoUrl ? shortenUrl(selectedLog.repoUrl) : "\u2014"}</span>
                            <span>{selectedLog.ref || "\u2014"}</span>
                            <span>{selectedLog.device}</span>
                            <BuildStatusBadge status={selectedLog.status} />
                            {selectedLog.error && (
                              <span style={{ color: "var(--danger)" }}>{selectedLog.error}</span>
                            )}
                          </div>
                        </div>
                        <button
                          onClick={() => {
                            setSelectedLog(null);
                            setSelectedLogError("");
                          }}
                          style={{
                            background: "none",
                            border: "1.5px solid var(--line)",
                            borderRadius: 8,
                            padding: "4px 12px",
                            fontSize: 12,
                            cursor: "pointer",
                            color: "var(--ink-muted)",
                          }}
                        >
                          {t.statsBuildLogsClose}
                        </button>
                      </div>
                      <div
                        style={{
                          flex: 1,
                          overflow: "auto",
                          background: "#1a1a2e",
                          borderRadius: 8,
                          padding: 12,
                        }}
                      >
                        <pre
                          style={{
                            margin: 0,
                            fontSize: 11,
                            lineHeight: 1.5,
                            fontFamily: "IBM Plex Mono, monospace",
                            color: "#e0e0e0",
                            whiteSpace: "pre-wrap",
                            wordBreak: "break-all",
                          }}
                        >
                          {selectedLog.lines.join("\n")}
                        </pre>
                      </div>
                    </>
                  )}
                </div>
              </div>
            )}

            {/* Recent events */}
            {data.recentEvents.length > 0 && (
              <div
                style={{
                  background: "var(--surface)",
                  borderRadius: "var(--radius)",
                  padding: 20,
                  boxShadow: "var(--shadow)",
                }}
              >
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", margin: "0 0 12px" }}>
                  <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                    {t.statsRecentEvents} ({data.recentEvents.length})
                  </h3>
                  <LimitSelector
                    label={t.statsShowLabel}
                    steps={RECENT_STEPS}
                    current={recentLimit}
                    loading={loading}
                    onChange={(v) => fetchStats({ recent: v })}
                    inline
                  />
                </div>
                <div style={{ overflowX: "auto" }}>
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
                    <thead>
                      <tr style={{ borderBottom: "1.5px solid var(--line)" }}>
                        {[t.statsEvTime, t.statsEvType, t.statsEvIP, t.statsEvRepo, t.statsEvRef, t.statsEvDevice].map((h) => (
                          <th
                            key={h}
                            style={{
                              padding: "6px 8px",
                              textAlign: "left",
                              color: "var(--ink-muted)",
                              fontWeight: 600,
                              whiteSpace: "nowrap",
                            }}
                          >
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {data.recentEvents.map((ev, i) => (
                        <tr key={i} style={{ borderBottom: "1px solid var(--line)" }}>
                          <td style={{ padding: "5px 8px", fontFamily: "IBM Plex Mono, monospace", whiteSpace: "nowrap" }}>
                            {formatTs(ev.ts, locale)}
                          </td>
                          <td style={{ padding: "5px 8px" }}>
                            <EventBadge type={ev.type} labels={eventLabelsI18n} />
                          </td>
                          <td style={{ padding: "5px 8px", fontFamily: "IBM Plex Mono, monospace" }}>{ev.ip}</td>
                          <td
                            style={{
                              padding: "5px 8px",
                              maxWidth: 200,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                            title={ev.repo}
                          >
                            {ev.repo ? shortenUrl(ev.repo) : "\u2014"}
                          </td>
                          <td
                            style={{
                              padding: "5px 8px",
                              fontFamily: "IBM Plex Mono, monospace",
                              maxWidth: 100,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                          >
                            {ev.ref || "\u2014"}
                          </td>
                          <td style={{ padding: "5px 8px" }}>{ev.device || ev.extra || "\u2014"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            <button
              onClick={() => setData(null)}
              style={{
                alignSelf: "flex-start",
                background: "none",
                border: "1.5px solid var(--line)",
                borderRadius: 10,
                padding: "8px 16px",
                fontSize: 13,
                cursor: "pointer",
                color: "var(--ink-muted)",
              }}
            >
              {t.statsChangePassword}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function StatsTable({ title, headers, rows }: { title: string; headers: string[]; rows: string[][] }) {
  return (
    <div
      style={{
        background: "var(--surface)",
        borderRadius: "var(--radius)",
        padding: 20,
        boxShadow: "var(--shadow)",
      }}
    >
      <h3 style={{ margin: "0 0 12px", fontSize: 15, fontWeight: 600 }}>{title}</h3>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
        <thead>
          <tr style={{ borderBottom: "1.5px solid var(--line)" }}>
            {headers.map((h, i) => (
              <th
                key={h}
                style={{
                  padding: "6px 8px",
                  textAlign: i === 0 ? "left" : "right",
                  color: "var(--ink-muted)",
                  fontWeight: 600,
                }}
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row[0]} style={{ borderBottom: "1px solid var(--line)" }}>
              {row.map((cell, j) => (
                <td
                  key={j}
                  style={{
                    padding: "6px 8px",
                    textAlign: j === 0 ? "left" : "right",
                    maxWidth: j === 0 ? 220 : undefined,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                  title={j === 0 ? cell : undefined}
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function LimitSelector({
  label,
  steps,
  current,
  loading,
  onChange,
  inline,
}: {
  label: string;
  steps: number[];
  current: number;
  loading: boolean;
  onChange: (v: number) => void;
  inline?: boolean;
}) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", ...(inline ? {} : { marginTop: -4 }) }}>
      <span style={{ fontSize: 12, color: "var(--ink-muted)" }}>{label}:</span>
      {steps.map((step) => (
        <button
          key={step}
          disabled={loading || current === step}
          onClick={() => onChange(step)}
          style={{
            background: current === step ? "var(--accent)" : "var(--surface)",
            color: current === step ? "#fff" : "var(--ink-muted)",
            border: current === step ? "none" : "1.5px solid var(--line)",
            borderRadius: 8,
            padding: "4px 10px",
            fontSize: 12,
            fontWeight: 600,
            cursor: loading || current === step ? "default" : "pointer",
            opacity: loading ? 0.6 : 1,
          }}
        >
          {step}
        </button>
      ))}
    </div>
  );
}

const eventColors: Record<string, string> = {
  visit: "#0f8f7f",
  discover: "#5c7cfa",
  build: "#f76707",
  download: "#2f9e44",
};

function EventBadge({ type, labels }: { type: string; labels: Record<string, string> }) {
  const color = eventColors[type] ?? "#888";
  return (
    <span
      style={{
        display: "inline-block",
        background: color + "22",
        color,
        borderRadius: 6,
        padding: "2px 7px",
        fontWeight: 600,
        fontSize: 11,
        whiteSpace: "nowrap",
      }}
    >
      {labels[type] ?? type}
    </span>
  );
}

function formatTs(ts: string, locale: Locale): string {
  const d = new Date(ts);
  if (isNaN(d.getTime())) {
    return ts;
  }
  return d.toLocaleString(locale === "ru" ? "ru-RU" : "en-GB", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function shortenUrl(url: string): string {
  try {
    return new URL(url).pathname.replace(/^\//, "") || url;
  } catch {
    return url;
  }
}

const statusColors: Record<string, string> = {
  success: "#2f9e44",
  failed: "#e03131",
  cancelled: "#868e96",
  running: "#f76707",
  queued: "#5c7cfa",
};

function BuildStatusBadge({ status }: { status: string }) {
  const color = statusColors[status] ?? "#888";
  return (
    <span
      style={{
        display: "inline-block",
        background: color + "22",
        color,
        borderRadius: 6,
        padding: "2px 7px",
        fontWeight: 600,
        fontSize: 11,
        whiteSpace: "nowrap",
      }}
    >
      {status}
    </span>
  );
}

function formatDuration(start: string, end: string): string {
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (isNaN(ms) || ms < 0) return "\u2014";
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainingSeconds}s`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours}h ${remainingMinutes}m`;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, i);
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[i]}`;
}
