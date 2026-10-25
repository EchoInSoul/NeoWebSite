import { cache } from "react";

import { backendUrl } from "@/lib/backend-url";
import type { CaptchaScope } from "@/lib/captcha-settings";

export type PublicSettingValue = boolean | number | string;

export type PublicSettings = Record<string, Record<string, PublicSettingValue>>;

function toSettings(payload: unknown): PublicSettings {
  const items = (payload as { items?: unknown } | null)?.items;
  if (!Array.isArray(items)) {
    throw new Error("public settings payload has no items array");
  }

  const settings: PublicSettings = {};
  for (const record of items as { group?: unknown; values?: unknown }[]) {
    if (typeof record?.group !== "string") {
      throw new Error("public settings payload has a group without a name");
    }
    if (typeof record.values !== "object" || record.values === null) {
      throw new Error(
        `public settings payload has malformed values for ${record.group}`,
      );
    }

    const values: Record<string, PublicSettingValue> = {};
    for (const [name, value] of Object.entries(record.values)) {
      if (
        typeof value === "boolean" ||
        typeof value === "number" ||
        typeof value === "string"
      ) {
        values[name] = value;
      }
    }
    settings[record.group] = values;
  }

  return settings;
}

export const getPublicSettings = cache(async (): Promise<PublicSettings> => {
  try {
    const response = await fetch(`${backendUrl}/api/v1/public/settings`, {
      next: { revalidate: 60 },
      signal: AbortSignal.timeout(5000),
    });
    if (!response.ok) {
      throw new Error(
        `public settings lookup failed: status ${response.status}`,
      );
    }
    return toSettings(await response.json());
  } catch {
    return {};
  }
});

export function isSignupOpen(settings: PublicSettings): boolean {
  return settings.register?.enabled !== false;
}

export type CaptchaConfig =
  | { provider: "turnstile"; siteKey: string }
  | { provider: "builtin"; scope: CaptchaScope };

export function getCaptchaConfig(
  settings: PublicSettings,
  scope: CaptchaScope,
): CaptchaConfig | null {
  const captcha = settings.captcha;

  if (captcha?.captcha_enabled !== true) {
    return null;
  }

  if (captcha.captcha_provider === "turnstile") {
    const siteKey = captcha.turnstile_site_key;

    if (typeof siteKey !== "string" || siteKey.trim() === "") {
      return null;
    }

    return { provider: "turnstile", siteKey };
  }

  if (captcha.captcha_provider === "builtin") {
    return { provider: "builtin", scope };
  }

  return null;
}
