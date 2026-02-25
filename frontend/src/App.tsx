import { FormEvent, useEffect, useRef, useState } from "react";
import {
  ArtifactItem,
  CaptchaChallenge,
  JobState,
  JobStatus,
  RepoRefsResponse,
  apiUrl,
  createBuildJob,
  createLogStream,
  discoverDevices,
  discoverRepoRefs,
  getCaptchaChallenge,
  getArtifacts,
  getJob,
  getServerHealth,
} from "./api";
import { Locale, dict } from "./i18n";

const finalStatuses = new Set<JobStatus>(["success", "failed", "cancelled"]);
const captchaSessionStorageKey = "mfb.captchaSessionToken";
const captchaBackendStorageKey = "mfb.captchaBackendBaseUrl";
const defaultRepoURL = "https://github.com/skrashevich/meshtastic-firmware";

export default function App() {
  const supportChatUrl = "https://t.me/meshtastic_firmware_builder";
  const supportChatRef = "t.me/meshtastic_firmware_builder";
  const projectRepoUrl = "https://github.com/skrashevich/meshtastic-firmware-builder";
  const projectRepoRef = "github.com/skrashevich/meshtastic-firmware-builder";

  const [locale, setLocale] = useState<Locale>("ru");
  const [repoUrl, setRepoUrl] = useState(defaultRepoURL);
  const [ref, setRef] = useState("");
  const [repoRefs, setRepoRefs] = useState<RepoRefsResponse | null>(null);
  const [refsLoading, setRefsLoading] = useState(false);
  const [refsError, setRefsError] = useState("");
  const [captcha, setCaptcha] = useState<CaptchaChallenge | null>(null);
  const [captchaAnswer, setCaptchaAnswer] = useState("");
  const [captchaLoading, setCaptchaLoading] = useState(false);
  const [captchaSessionToken, setCaptchaSessionToken] = useState("");
  const [captchaBackendBaseUrl, setCaptchaBackendBaseUrl] = useState("");
  const [captchaRequired, setCaptchaRequired] = useState(true);
  const [devices, setDevices] = useState<string[]>([]);
  const [selectedDevice, setSelectedDevice] = useState("");

  const [discovering, setDiscovering] = useState(false);
  const [startingBuild, setStartingBuild] = useState(false);

  const [job, setJob] = useState<JobState | null>(null);
  const [jobBackendBaseUrl, setJobBackendBaseUrl] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [artifacts, setArtifacts] = useState<ArtifactItem[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [error, setError] = useState("");

  const streamRef = useRef<EventSource | null>(null);
  const refsRepoRef = useRef("");
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
    const trimmedRepo = repoUrl.trim();
    if (!trimmedRepo || !looksLikeRepoURL(trimmedRepo)) {
      setRepoRefs(null);
      setRefsError("");
      if (!trimmedRepo) {
        refsRepoRef.current = "";
      }
      return;
    }

    const controller = new AbortController();
    const timeoutID = window.setTimeout(async () => {
      setRefsLoading(true);
      setRefsError("");
      try {
        const refsData = await discoverRepoRefs(trimmedRepo, controller.signal);
        setRepoRefs(refsData);

        const repoChanged = refsRepoRef.current !== trimmedRepo;
        refsRepoRef.current = trimmedRepo;
        if (repoChanged && refsData.defaultBranch) {
          setRef(refsData.defaultBranch);
        }
      } catch (requestError) {
        if (controller.signal.aborted) {
          return;
        }
        setRepoRefs(null);
        setRefsError(errorToMessage(requestError, t.unknownError));
      } finally {
        if (!controller.signal.aborted) {
          setRefsLoading(false);
        }
      }
    }, 350);

    return () => {
      controller.abort();
      window.clearTimeout(timeoutID);
    };
  }, [repoUrl, t.unknownError]);

  useEffect(() => {
    let cancelled = false;
    const storedSessionToken = (window.sessionStorage.getItem(captchaSessionStorageKey) ?? "").trim();
    const storedSessionBackend = (window.sessionStorage.getItem(captchaBackendStorageKey) ?? "").trim();
    if (storedSessionToken) {
      setCaptchaSessionToken(storedSessionToken);
    }
    if (storedSessionBackend) {
      setCaptchaBackendBaseUrl(storedSessionBackend);
    }

    const bootstrap = async () => {
      try {
        const health = await getServerHealth(storedSessionBackend || undefined, saveCaptchaBackendBaseUrl);
        if (cancelled) {
          return;
        }

        const requiresCaptcha = health.captchaRequired !== false;
        setCaptchaRequired(requiresCaptcha);

        if (!requiresCaptcha) {
          clearCaptchaSessionToken();
          setCaptcha(null);
          setCaptchaAnswer("");
          return;
        }

        if (!storedSessionToken) {
          await refreshCaptcha(storedSessionBackend || undefined);
        }
      } catch {
        if (cancelled) {
          return;
        }

        setCaptchaRequired(true);
        if (!storedSessionToken) {
          await refreshCaptcha(storedSessionBackend || undefined);
        }
      }
    };

    void bootstrap();

    return () => {
      cancelled = true;
    };
  }, []);

  function saveCaptchaSessionToken(token: string, backendBaseUrl?: string) {
    const value = token.trim();
    if (!value) {
      return;
    }

    if (backendBaseUrl) {
      saveCaptchaBackendBaseUrl(backendBaseUrl);
    }

    setCaptchaSessionToken(value);
    window.sessionStorage.setItem(captchaSessionStorageKey, value);
  }

  function saveCaptchaBackendBaseUrl(value: string) {
    const normalized = value.trim();
    if (!normalized) {
      return;
    }

    setCaptchaBackendBaseUrl(normalized);
    window.sessionStorage.setItem(captchaBackendStorageKey, normalized);
  }

  function saveJobBackendBaseUrl(value: string) {
    const normalized = value.trim();
    if (!normalized) {
      return;
    }

    setJobBackendBaseUrl((current) => (current === normalized ? current : normalized));
  }

  function clearCaptchaSessionToken() {
    setCaptchaSessionToken("");
    window.sessionStorage.removeItem(captchaSessionStorageKey);
    setCaptchaBackendBaseUrl("");
    window.sessionStorage.removeItem(captchaBackendStorageKey);
  }

  async function refreshCaptcha(preferredBackendBaseUrl?: string) {
    if (!captchaRequired) {
      return;
    }

    setCaptchaLoading(true);
    try {
      const challenge = await getCaptchaChallenge(
        preferredBackendBaseUrl || captchaBackendBaseUrl || undefined,
        saveCaptchaBackendBaseUrl,
      );

      if (challenge.captchaRequired === false) {
        setCaptchaRequired(false);
        clearCaptchaSessionToken();
        setCaptcha(null);
        setCaptchaAnswer("");
        return;
      }

      setCaptcha(challenge);
      setCaptchaAnswer("");
    } catch (requestError) {
      setCaptcha(null);
      setError(errorToMessage(requestError, t.unknownError));
    } finally {
      setCaptchaLoading(false);
    }
  }

  function applyRefChoice(value: string) {
    setRef(value);
  }

  useEffect(() => {
    if (!job?.id) {
      return;
    }

    const intervalId = window.setInterval(async () => {
      try {
        let currentBackendBaseUrl = jobBackendBaseUrl;
        const rememberBackend = (backendBaseUrl: string) => {
          currentBackendBaseUrl = backendBaseUrl;
          saveJobBackendBaseUrl(backendBaseUrl);
        };

        const current = await getJob(job.id, currentBackendBaseUrl || undefined, rememberBackend);
        setJob(current);
        if (current.status === "success") {
          const files = await getArtifacts(current.id, currentBackendBaseUrl || undefined, rememberBackend);
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
  }, [job?.id, jobBackendBaseUrl, t.unknownError]);

  async function onDiscoverSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");

    if (!repoUrl.trim()) {
      setError(t.repoRequired);
      return;
    }

    const hasCaptchaSession = captchaSessionToken.trim() !== "";
    if (captchaRequired && !hasCaptchaSession && (!captcha?.captchaId || !captchaAnswer.trim())) {
      setError(t.captchaRequired);
      return;
    }

    setDiscovering(true);
    try {
      const preferredBackend = captchaBackendBaseUrl || undefined;
      let resolvedBackendBaseUrl = preferredBackend ?? "";
      const rememberCaptchaBackend = (backendBaseUrl: string) => {
        resolvedBackendBaseUrl = backendBaseUrl;
        saveCaptchaBackendBaseUrl(backendBaseUrl);
      };

      const result = hasCaptchaSession
        ? await discoverDevices(
            repoUrl.trim(),
            ref.trim(),
            undefined,
            undefined,
            captchaSessionToken,
            preferredBackend,
            rememberCaptchaBackend,
          )
        : await discoverDevices(
            repoUrl.trim(),
            ref.trim(),
            captcha?.captchaId,
            captchaAnswer.trim(),
            undefined,
            preferredBackend,
            rememberCaptchaBackend,
          );

      if (result.captchaSessionToken) {
        saveCaptchaSessionToken(result.captchaSessionToken, resolvedBackendBaseUrl);
      }
      setDevices(result.devices);
      setSelectedDevice(result.devices[0] ?? "");
      setJob(null);
      setJobBackendBaseUrl("");
      setArtifacts([]);
      setLogs([]);
      closeStream();
    } catch (requestError) {
      const message = errorToMessage(requestError, t.unknownError);
      setError(message);

      if (captchaRequired && message.toLowerCase().includes("captcha")) {
        clearCaptchaSessionToken();
      }

      if (captchaRequired && (!hasCaptchaSession || message.toLowerCase().includes("captcha"))) {
        void refreshCaptcha(captchaBackendBaseUrl || undefined);
      }
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

    const hasCaptchaSession = captchaSessionToken.trim() !== "";
    if (captchaRequired && !hasCaptchaSession && (!captcha?.captchaId || !captchaAnswer.trim())) {
      setError(t.captchaRequired);
      return;
    }

    setStartingBuild(true);
    try {
      const preferredBackend = captchaBackendBaseUrl || undefined;
      let resolvedBackendBaseUrl = preferredBackend ?? "";
      const rememberBuildBackend = (backendBaseUrl: string) => {
        resolvedBackendBaseUrl = backendBaseUrl;
        saveCaptchaBackendBaseUrl(backendBaseUrl);
        saveJobBackendBaseUrl(backendBaseUrl);
      };

      const created = hasCaptchaSession
        ? await createBuildJob(
            repoUrl.trim(),
            ref.trim(),
            selectedDevice,
            undefined,
            undefined,
            captchaSessionToken,
            preferredBackend,
            rememberBuildBackend,
          )
        : await createBuildJob(
            repoUrl.trim(),
            ref.trim(),
            selectedDevice,
            captcha?.captchaId,
            captchaAnswer.trim(),
            undefined,
            preferredBackend,
            rememberBuildBackend,
          );

      if (created.captchaSessionToken) {
        saveCaptchaSessionToken(created.captchaSessionToken, resolvedBackendBaseUrl);
      }

      setJob(created);
      setArtifacts([]);
      setLogs([]);
      openStream(created.id, resolvedBackendBaseUrl || preferredBackend);
    } catch (requestError) {
      const message = errorToMessage(requestError, t.unknownError);
      setError(message);

      if (captchaRequired && message.toLowerCase().includes("captcha")) {
        clearCaptchaSessionToken();
        void refreshCaptcha();
      }
    } finally {
      setStartingBuild(false);
    }
  }

  function openStream(jobId: string, backendBaseUrl?: string) {
    closeStream();

    const stream = createLogStream(jobId, backendBaseUrl, saveJobBackendBaseUrl);
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
  const defaultBranchNote =
    repoRefs?.defaultBranch && repoRefs.defaultBranch.trim()
      ? t.refsDefaultBranch.replace("{branch}", repoRefs.defaultBranch)
      : "";
  const refsErrorNote = refsError ? t.refsLoadFailed.replace("{error}", refsError) : "";
  const branchSuggestions = repoRefs ? limitRefItems(repoRefs.recentBranches, 8) : [];
  const tagSuggestions = repoRefs ? limitRefItems(repoRefs.recentTags, 8) : [];
  const refInputSuggestions = collectRefSuggestions(repoRefs);

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
                list="repo-ref-options"
              />
            </label>

            <datalist id="repo-ref-options">
              {refInputSuggestions.map((refName) => (
                <option key={refName} value={refName} />
              ))}
            </datalist>

            {refsLoading ? <p className="muted refs-meta">{t.refsLoading}</p> : null}
            {defaultBranchNote ? <p className="muted refs-meta">{defaultBranchNote}</p> : null}
            {refsErrorNote ? <p className="ref-error">{refsErrorNote}</p> : null}

            {branchSuggestions.length > 0 ? (
              <div className="ref-group">
                <p>{t.refsRecentBranches}</p>
                <div className="ref-chips">
                  {branchSuggestions.map((branch) => (
                    <button
                      key={`branch-${branch.name}`}
                      className={ref === branch.name ? "ref-chip active" : "ref-chip"}
                      type="button"
                      onClick={() => applyRefChoice(branch.name)}
                      title={branch.updatedAt ?? branch.name}
                    >
                      {branch.name}
                    </button>
                  ))}
                </div>
              </div>
            ) : null}

            {tagSuggestions.length > 0 ? (
              <div className="ref-group">
                <p>{t.refsRecentTags}</p>
                <div className="ref-chips">
                  {tagSuggestions.map((tag) => (
                    <button
                      key={`tag-${tag.name}`}
                      className={ref === tag.name ? "ref-chip active" : "ref-chip"}
                      type="button"
                      onClick={() => applyRefChoice(tag.name)}
                      title={tag.updatedAt ?? tag.name}
                    >
                      {tag.name}
                    </button>
                  ))}
                </div>
              </div>
            ) : null}

            {!captchaRequired ? (
              <p className="muted refs-meta">{t.captchaDisabled}</p>
            ) : captchaSessionToken ? (
              <p className="muted refs-meta">{t.captchaSessionActive}</p>
            ) : (
              <label>
                <span>{t.captchaLabel}</span>
                <div className="captcha-row">
                  <div className="captcha-question" title={t.captchaTooltip}>
                    {captchaLoading ? t.captchaLoading : captcha?.question ?? t.captchaLoading}
                  </div>
                  <button className="ghost" type="button" onClick={() => void refreshCaptcha()} disabled={captchaLoading}>
                    {t.captchaRefresh}
                  </button>
                </div>
                <input
                  value={captchaAnswer}
                  onChange={(event) => setCaptchaAnswer(event.target.value)}
                  placeholder={t.captchaPlaceholder}
                />
              </label>
            )}

            <button
              className="primary"
              type="submit"
              disabled={
                discovering ||
                (captchaRequired && !captchaSessionToken && (captchaLoading || !captcha || !captchaAnswer.trim()))
              }
            >
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
              disabled={
                !selectedDevice ||
                startingBuild ||
                (captchaRequired && !captchaSessionToken && (captchaLoading || !captcha || !captchaAnswer.trim()))
              }
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
                  <a href={apiUrl(artifact.downloadUrl, jobBackendBaseUrl || undefined)} target="_blank" rel="noreferrer">
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

        <footer className="site-footer">
          <span>
            {t.footerAuthor}: Sergei "svk" Krashevich
          </span>
          <span>
            {t.footerRepository}: {" "}
            <a href={projectRepoUrl} target="_blank" rel="noreferrer">
              {projectRepoRef}
            </a>
          </span>
        </footer>
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

function collectRefSuggestions(repoRefs: RepoRefsResponse | null): string[] {
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

function limitRefItems<T extends { name: string }>(items: T[], limit: number): T[] {
  if (limit < 1 || items.length <= limit) {
    return items;
  }
  return items.slice(0, limit);
}

function looksLikeRepoURL(value: string): boolean {
  return /^(https?:\/\/|ssh:\/\/|git:\/\/|git@)/.test(value);
}
