import ru from "./locales/ru.json";
import en from "./locales/en.json";

export type Locale = "ru" | "en";
export type Dictionary = typeof ru;
export const dict: Record<Locale, Dictionary> = { ru, en };
