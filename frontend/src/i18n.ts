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
  discover: string;
  discovering: string;
  devicesTitle: string;
  deviceLabel: string;
  startBuild: string;
  startingBuild: string;
  status: string;
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
    discover: "Найти устройства",
    discovering: "Поиск устройств...",
    devicesTitle: "Доступные устройства (каталог variants)",
    deviceLabel: "Устройство",
    startBuild: "Запустить сборку",
    startingBuild: "Запуск...",
    status: "Статус",
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
    supportTone: "Мы приветствуем вопросы и просьбы, но не требования.",
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
    discover: "Discover devices",
    discovering: "Discovering devices...",
    devicesTitle: "Available devices (variants directory)",
    deviceLabel: "Device",
    startBuild: "Start build",
    startingBuild: "Starting...",
    status: "Status",
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
