import { FormEvent, useEffect, useRef, useState } from "react";
import {
  ArtifactItem,
  JobState,
  JobStatus,
  apiUrl,
  createBuildJob,
  createLogStream,
  discoverDevices,
  getArtifacts,
  getJob,
} from "./api";
import { Locale, dict } from "./i18n";

const finalStatuses = new Set<JobStatus>(["success", "failed", "cancelled"]);

export default function App() {
  const supportChatUrl = "https://t.me/meshtastic_firmware_builder";
  const supportChatRef = "t.me/meshtastic_firmware_builder";

  const [locale, setLocale] = useState<Locale>("ru");
  const [repoUrl, setRepoUrl] = useState("");
  const [ref, setRef] = useState("");
  const [devices, setDevices] = useState<string[]>([]);
  const [selectedDevice, setSelectedDevice] = useState("");

  const [discovering, setDiscovering] = useState(false);
  const [startingBuild, setStartingBuild] = useState(false);

  const [job, setJob] = useState<JobState | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [artifacts, setArtifacts] = useState<ArtifactItem[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [error, setError] = useState("");

  const streamRef = useRef<EventSource | null>(null);
  const logsTailRef = useRef<HTMLDivElement | null>(null);

  const t = dict[locale];

  useEffect(() => {
    return () => {
      if (streamRef.current) {
        streamRef.current.close();
      }
    };
  }, []);

  useEffect(() => {
    if (!autoScroll) {
      return;
    }
    logsTailRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs, autoScroll]);

  useEffect(() => {
    if (!job?.id) {
      return;
    }

    const intervalId = window.setInterval(async () => {
      try {
        const current = await getJob(job.id);
        setJob(current);
        if (current.status === "success") {
          const files = await getArtifacts(current.id);
          setArtifacts(files);
        }
        if (finalStatuses.has(current.status)) {
          window.clearInterval(intervalId);
          closeStream();
        }
      } catch (requestError) {
        setError(errorToMessage(requestError, t.unknownError));
        window.clearInterval(intervalId);
      }
    }, 2000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [job?.id]);

  async function onDiscoverSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");

    if (!repoUrl.trim()) {
      setError(t.repoRequired);
      return;
    }

    setDiscovering(true);
    try {
      const result = await discoverDevices(repoUrl.trim(), ref.trim());
      setDevices(result.devices);
      setSelectedDevice(result.devices[0] ?? "");
      setJob(null);
      setArtifacts([]);
      setLogs([]);
      closeStream();
    } catch (requestError) {
      setError(errorToMessage(requestError, t.unknownError));
    } finally {
      setDiscovering(false);
    }
  }

  async function onStartBuild() {
    setError("");
    if (!selectedDevice) {
      setError(t.chooseDevice);
      return;
    }

    setStartingBuild(true);
    try {
      const created = await createBuildJob(repoUrl.trim(), ref.trim(), selectedDevice);
      setJob(created);
      setArtifacts([]);
      setLogs([]);
      openStream(created.id);
    } catch (requestError) {
      setError(errorToMessage(requestError, t.unknownError));
    } finally {
      setStartingBuild(false);
    }
  }

  function openStream(jobId: string) {
    closeStream();

    const stream = createLogStream(jobId);
    stream.addEventListener("log", (event) => {
      const message = event as MessageEvent<string>;
      setLogs((current) => [...current, message.data]);
    });
    stream.onerror = () => {
      stream.close();
      if (streamRef.current === stream) {
        streamRef.current = null;
      }
    };
    streamRef.current = stream;
  }

  function closeStream() {
    if (!streamRef.current) {
      return;
    }
    streamRef.current.close();
    streamRef.current = null;
  }

  const statusLabel = job ? t.statuses[job.status] ?? job.status : "-";
  const queueNote =
    job?.status === "queued"
      ? typeof job.queuePosition === "number" && job.queuePosition > 0
        ? t.queueInfoWithPos.replace("{position}", String(job.queuePosition))
        : t.queueInfo
      : "";
  const queueEtaNote =
    job?.status === "queued" && typeof job.queueEtaSeconds === "number" && job.queueEtaSeconds > 0
      ? t.queueEta.replace("{eta}", formatQueueETA(job.queueEtaSeconds, locale))
      : "";
  const supportIntroParts = t.supportIntro.split("{chat}");

  return (
    <div className="page-shell">
      <div className="bg-orb orb-a" />
      <div className="bg-orb orb-b" />

      <main className="layout">
        <header className="hero panel">
          <div>
            <p className="badge">Meshtastic</p>
            <h1>{t.title}</h1>
            <p className="subtitle">{t.subtitle}</p>
          </div>

          <div className="locale-box">
            <span>{t.language}</span>
            <div className="locale-actions">
              <button
                className={locale === "ru" ? "locale-btn active" : "locale-btn"}
                onClick={() => setLocale("ru")}
                type="button"
              >
                RU
              </button>
              <button
                className={locale === "en" ? "locale-btn active" : "locale-btn"}
                onClick={() => setLocale("en")}
                type="button"
              >
                EN
              </button>
            </div>
          </div>
        </header>

        <section className="panel form-panel reveal-1">
          <form className="grid-form" onSubmit={onDiscoverSubmit}>
            <label>
              <span>{t.repoLabel}</span>
              <input
                value={repoUrl}
                onChange={(event) => setRepoUrl(event.target.value)}
                placeholder={t.repoPlaceholder}
                required
              />
            </label>

            <label>
              <span>{t.refLabel}</span>
              <input
                value={ref}
                onChange={(event) => setRef(event.target.value)}
                placeholder={t.refPlaceholder}
              />
            </label>

            <button className="primary" type="submit" disabled={discovering}>
              {discovering ? t.discovering : t.discover}
            </button>
          </form>
        </section>

        <section className="panel reveal-2">
          <div className="panel-head">
            <h2>{t.devicesTitle}</h2>
            <span className="status-chip">
              {t.status}: <strong>{statusLabel}</strong>
            </span>
          </div>

          <div className="devices-grid">
            {devices.length === 0 ? <p className="muted">{t.noDevices}</p> : null}
            {devices.map((device) => (
              <label key={device} className={selectedDevice === device ? "device-card active" : "device-card"}>
                <input
                  type="radio"
                  name="device"
                  value={device}
                  checked={selectedDevice === device}
                  onChange={() => setSelectedDevice(device)}
                />
                <span>{device}</span>
              </label>
            ))}
          </div>

          <div className="actions-row">
            <button
              className="primary"
              onClick={onStartBuild}
              type="button"
              disabled={!selectedDevice || startingBuild}
            >
              {startingBuild ? t.startingBuild : t.startBuild}
            </button>
            <button className="ghost" type="button" onClick={() => setLogs([])}>
              {t.clearLogs}
            </button>
            <button className="ghost" type="button" onClick={() => setAutoScroll((value) => !value)}>
              {autoScroll ? t.autoScrollOn : t.autoScrollOff}
            </button>
          </div>
        </section>

        <section className="panel logs-panel reveal-3">
          <div className="panel-head">
            <h2>{t.logs}</h2>
            <p>{t.logsHint}</p>
          </div>
          <pre className="logs-box">
            {queueNote || queueEtaNote ? (
              <span className="logs-queue-note-wrap">
                {queueNote ? <span className="logs-queue-note">{queueNote}</span> : null}
                {queueEtaNote ? <span className="logs-queue-note">{queueEtaNote}</span> : null}
              </span>
            ) : null}
            {logs.join("\n")}
            <div ref={logsTailRef} />
          </pre>
        </section>

        <section className="panel reveal-4">
          <div className="panel-head">
            <h2>{t.artifacts}</h2>
          </div>
          {artifacts.length === 0 ? (
            <p className="muted">{t.noArtifacts}</p>
          ) : (
            <ul className="artifacts-list">
              {artifacts.map((artifact) => (
                <li key={artifact.id}>
                  <a href={apiUrl(artifact.downloadUrl)} target="_blank" rel="noreferrer">
                    {artifact.relativePath}
                  </a>
                  <span>{formatSize(artifact.size)}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel support-panel">
          <div className="panel-head">
            <h2>{t.supportTitle}</h2>
          </div>
          <p className="support-note">
            {supportIntroParts[0]}
            <a href={supportChatUrl} target="_blank" rel="noreferrer">
              {supportChatRef}
            </a>
            {supportIntroParts[1] ?? ""}
          </p>
          <p className="support-note">{t.supportDisclaimer}</p>
          <p className="support-note">{t.supportTone}</p>
        </section>

        {error ? <section className="error-banner">{error}</section> : null}
      </main>
    </div>
  );
}

function errorToMessage(value: unknown, fallback: string): string {
  if (value instanceof Error) {
    return value.message;
  }
  return fallback;
}

function formatSize(size: number): string {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function formatQueueETA(seconds: number, locale: Locale): string {
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
