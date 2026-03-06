import { FormEvent, useState } from "react";
import { StatsSummary, getStats } from "./api";

const RECENT_STEPS = [50, 150, 500];
const TOP_STEPS = [10, 30, 100];

export default function StatsPage() {
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [data, setData] = useState<StatsSummary | null>(null);
  const [recentLimit, setRecentLimit] = useState(RECENT_STEPS[0]);
  const [topLimit, setTopLimit] = useState(TOP_STEPS[0]);

  async function fetchStats(opts?: { recent?: number; top?: number }) {
    const rl = opts?.recent ?? recentLimit;
    const tl = opts?.top ?? topLimit;
    setLoading(true);
    setError("");
    try {
      const summary = await getStats(password, { recentLimit: rl, topLimit: tl });
      setData(summary);
      setRecentLimit(rl);
      setTopLimit(tl);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Ошибка запроса");
    } finally {
      setLoading(false);
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    await fetchStats();
  }

  return (
    <div className="page-shell">
      <div className="layout" style={{ maxWidth: 960, margin: "0 auto", padding: "28px 16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 8 }}>
          <a href="/" style={{ color: "var(--accent)", textDecoration: "none", fontSize: 14 }}>
            ← Назад
          </a>
          <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700 }}>Статистика использования</h1>
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
              <label style={{ fontWeight: 600, fontSize: 14 }}>Пароль для просмотра статистики</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Введите пароль..."
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
                {loading ? "Загрузка..." : "Показать"}
              </button>
            </form>
          </div>
        )}

        {data && (
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {/* Counters */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: 12 }}>
              {[
                { label: "Визиты", value: data.totalVisits, color: "#0f8f7f" },
                { label: "Поиск устройств", value: data.totalDiscovers, color: "#5c7cfa" },
                { label: "Сборки", value: data.totalBuilds, color: "#f76707" },
                { label: "Скачивания", value: data.totalDownloads, color: "#2f9e44" },
                { label: "Уникальных IP", value: data.uniqueIPs, color: "#ae3ec9" },
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
                  title={`Топ репозиториев (${data.topRepos.length})`}
                  headers={["Репозиторий", "Запросов"]}
                  rows={data.topRepos.map((r) => [shortenUrl(r.name), String(r.count)])}
                />
              )}

              {/* Top devices */}
              {data.topDevices.length > 0 && (
                <StatsTable
                  title={`Топ устройств (${data.topDevices.length})`}
                  headers={["Устройство", "Сборок"]}
                  rows={data.topDevices.map((d) => [d.name, String(d.count)])}
                />
              )}
            </div>

            {(data.topRepos.length > 0 || data.topDevices.length > 0) && (
              <LimitSelector
                label="Топ"
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
                <h3 style={{ margin: "0 0 12px", fontSize: 15, fontWeight: 600 }}>По дням</h3>
                <div style={{ overflowX: "auto" }}>
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                    <thead>
                      <tr style={{ borderBottom: "1.5px solid var(--line)" }}>
                        {["Дата", "Визиты", "Поиск", "Сборки", "Скачивания"].map((h) => (
                          <th
                            key={h}
                            style={{
                              padding: "6px 10px",
                              textAlign: h === "Дата" ? "left" : "right",
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
                    Последние события ({data.recentEvents.length})
                  </h3>
                  <LimitSelector
                    label="Показать"
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
                        {["Время", "Тип", "IP", "Репозиторий", "Ref", "Устройство"].map((h) => (
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
                            {formatTs(ev.ts)}
                          </td>
                          <td style={{ padding: "5px 8px" }}>
                            <EventBadge type={ev.type} />
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
                            {ev.repo ? shortenUrl(ev.repo) : "—"}
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
                            {ev.ref || "—"}
                          </td>
                          <td style={{ padding: "5px 8px" }}>{ev.device || ev.extra || "—"}</td>
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
              Сменить пароль
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

const eventLabels: Record<string, string> = {
  visit: "визит",
  discover: "поиск",
  build: "сборка",
  download: "скачать",
};

function EventBadge({ type }: { type: string }) {
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
      {eventLabels[type] ?? type}
    </span>
  );
}

function formatTs(ts: string): string {
  const d = new Date(ts);
  if (isNaN(d.getTime())) {
    return ts;
  }
  return d.toLocaleString("ru-RU", {
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
