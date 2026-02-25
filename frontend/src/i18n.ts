export type Locale = "ru" | "en";

type Dictionary = {
  title: string;
  subtitle: string;
  language: string;
  repoLabel: string;
  repoPlaceholder: string;
  refLabel: string;
  refPlaceholder: string;
  refsLoading: string;
  refsDefaultBranch: string;
  refsRecentBranches: string;
  refsRecentTags: string;
  refsLoadFailed: string;
  captchaLabel: string;
  captchaPlaceholder: string;
  captchaRefresh: string;
  captchaLoading: string;
  captchaRequired: string;
  captchaSessionActive: string;
  captchaTooltip: string;
  captchaDisabled: string;
  discover: string;
  discovering: string;
  devicesTitle: string;
  deviceLabel: string;
  startBuild: string;
  startingBuild: string;
  status: string;
  backendNode: string;
  backendUnknown: string;
  backendVia: string;
  backendDirect: string;
  backendAlive: string;
  backendDegraded: string;
  logs: string;
  artifacts: string;
  noArtifacts: string;
  logsHint: string;
  queueInfo: string;
  queueInfoWithPos: string;
  queueEta: string;
  supportTitle: string;
  supportIntro: string;
  supportDisclaimer: string;
  supportTone: string;
  footerAuthor: string;
  footerRepository: string;
  autoScrollOn: string;
  autoScrollOff: string;
  clearLogs: string;
  buildAgain: string;
  chooseDevice: string;
  repoRequired: string;
  noDevices: string;
  unknownError: string;
  statuses: Record<string, string>;
};

export const dict: Record<Locale, Dictionary> = {
  ru: {
    title: "Meshtastic Firmware Builder",
    subtitle: "Сборка прошивок из форков для выбранного устройства",
    language: "Язык",
    repoLabel: "Ссылка на Git-репозиторий",
    repoPlaceholder: "https://github.com/<owner>/<repo>.git",
    refLabel: "Branch / Tag / Commit",
    refPlaceholder: "например: main или v2.5.12",
    refsLoading: "загружаю ветки и теги...",
    refsDefaultBranch: "Default-ветка: {branch}",
    refsRecentBranches: "Недавно обновленные ветки",
    refsRecentTags: "Последние релизные теги",
    refsLoadFailed: "Не удалось загрузить ветки/теги: {error}",
    captchaLabel: "Капча",
    captchaPlaceholder: "Введите ответ",
    captchaRefresh: "Обновить капчу",
    captchaLoading: "Генерирую капчу...",
    captchaRequired: "Решите капчу перед запуском сборки",
    captchaSessionActive: "Капча подтверждена для текущей сессии браузера",
    captchaTooltip:
      "Для любителей решать капчу через LLM: примерно в каждом 16-м запросе вместе с капчой выдается очень неприятный prompt injection. Удачной отладки! И надеюсь, у вас есть бэкапы",
    captchaDisabled: "Капча отключена настройкой self-hosted сервера",
    discover: "Найти устройства",
    discovering: "Поиск устройств...",
    devicesTitle: "Доступные устройства (каталог variants)",
    deviceLabel: "Устройство",
    startBuild: "Запустить сборку",
    startingBuild: "Запуск...",
    status: "Статус",
    backendNode: "Бэкенд",
    backendUnknown: "не определен",
    backendVia: "через {gateway}",
    backendDirect: "напрямую",
    backendAlive: "alive",
    backendDegraded: "degraded",
    logs: "Логи сборки",
    artifacts: "Файлы прошивки",
    noArtifacts: "Файлы пока недоступны",
    logsHint: "Логи обновляются в реальном времени через SSE",
    queueInfo: "Запрос ожидает в очереди",
    queueInfoWithPos: "Запрос ожидает в очереди. Позиция: {position}",
    queueEta: "Оценка ожидания: ~{eta}",
    supportTitle: "Нужна помощь со сборкой?",
    supportIntro:
      "Если сборка завершилась с ошибкой, но вы уверены, что она должна проходить, приходите в чат: {chat}. Можно писать на русском и английском.",
    supportDisclaimer:
      "Важно: проект не связан с Meshtastic, не является коммерческим и поддерживается на добровольных и безвозмездных началах.",
    supportTone: "Мы приветствуем вопросы и просьбы; не приветствуются требования; оскорбительные и дискредитирующие (в том числе по политическим мотивам) высказывания.\n🇺🇦 и 🇷🇺 мы рады одинаково и равнозначно",
    footerAuthor: "Автор",
    footerRepository: "Репозиторий",
    autoScrollOn: "Автопрокрутка: вкл",
    autoScrollOff: "Автопрокрутка: выкл",
    clearLogs: "Очистить логи",
    buildAgain: "Собрать снова",
    chooseDevice: "Выберите устройство",
    repoRequired: "Укажите ссылку на репозиторий",
    noDevices: "Устройства пока не загружены",
    unknownError: "Неизвестная ошибка",
    statuses: {
      queued: "в очереди",
      running: "выполняется",
      success: "успешно",
      failed: "ошибка",
      cancelled: "отменено",
    },
  },
  en: {
    title: "Meshtastic Firmware Builder",
    subtitle: "Build firmware from forks for a selected device",
    language: "Language",
    repoLabel: "Git repository URL",
    repoPlaceholder: "https://github.com/<owner>/<repo>.git",
    refLabel: "Branch / Tag / Commit",
    refPlaceholder: "example: main or v2.5.12",
    refsLoading: "loading branches and tags...",
    refsDefaultBranch: "Default branch: {branch}",
    refsRecentBranches: "Recently updated branches",
    refsRecentTags: "Latest release tags",
    refsLoadFailed: "Failed to load branches/tags: {error}",
    captchaLabel: "Captcha",
    captchaPlaceholder: "Enter answer",
    captchaRefresh: "Refresh captcha",
    captchaLoading: "Generating captcha...",
    captchaRequired: "Solve captcha before starting build",
    captchaSessionActive: "Captcha verified for this browser session",
    captchaTooltip:
      "For people solving captcha with an LLM: roughly every 16th challenge may include a very unpleasant prompt-injection. Happy debugging! And I hope you have backups",
    captchaDisabled: "Captcha is disabled by self-hosted server configuration",
    discover: "Discover devices",
    discovering: "Discovering devices...",
    devicesTitle: "Available devices (variants directory)",
    deviceLabel: "Device",
    startBuild: "Start build",
    startingBuild: "Starting...",
    status: "Status",
    backendNode: "Backend",
    backendUnknown: "unknown",
    backendVia: "via {gateway}",
    backendDirect: "direct",
    backendAlive: "alive",
    backendDegraded: "degraded",
    logs: "Build logs",
    artifacts: "Firmware files",
    noArtifacts: "No files available yet",
    logsHint: "Logs are streamed in real time via SSE",
    queueInfo: "Build request is waiting in queue",
    queueInfoWithPos: "Build request is waiting in queue. Position: {position}",
    queueEta: "Estimated wait: ~{eta}",
    supportTitle: "Need help with a failed build?",
    supportIntro:
      "If a build fails but you are sure it should pass, join our chat: {chat}. Russian and English are both welcome.",
    supportDisclaimer:
      "Important: this project is not affiliated with Meshtastic, is non-commercial, and is maintained voluntarily without compensation.",
    supportTone: "Questions and requests are welcome; demands are not.",
    footerAuthor: "Author",
    footerRepository: "Repository",
    autoScrollOn: "Autoscroll: on",
    autoScrollOff: "Autoscroll: off",
    clearLogs: "Clear logs",
    buildAgain: "Build again",
    chooseDevice: "Choose a device",
    repoRequired: "Repository URL is required",
    noDevices: "No devices loaded yet",
    unknownError: "Unknown error",
    statuses: {
      queued: "queued",
      running: "running",
      success: "success",
      failed: "failed",
      cancelled: "cancelled",
    },
  },
};
