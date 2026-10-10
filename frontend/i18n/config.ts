export const LOCALE_COOKIE_NAME = "locale";

export const SUPPORTED_LOCALES = ["zh-CN", "en"] as const;

export type Locale = (typeof SUPPORTED_LOCALES)[number];

export const DEFAULT_LOCALE: Locale = "zh-CN";

export function isLocale(value: string | undefined): value is Locale {
  return value === "zh-CN" || value === "en";
}
