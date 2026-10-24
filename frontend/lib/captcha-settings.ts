export type CaptchaSettingsValues = {
  captchaEnabled: boolean;
  captchaProvider: string;
  captchaScopes: string;
  turnstileSiteKey: string;
  turnstileSecretKey: string;
};

export const CAPTCHA_PROVIDER_NONE = "none";
export const CAPTCHA_PROVIDER_BUILTIN = "builtin";
export const CAPTCHA_PROVIDER_TURNSTILE = "turnstile";

export const CAPTCHA_SCOPE_LOGIN = "login";
export const CAPTCHA_SCOPE_REGISTER = "register";

export const CAPTCHA_SCOPE_ORDER = [
  CAPTCHA_SCOPE_LOGIN,
  CAPTCHA_SCOPE_REGISTER,
] as const;

export type CaptchaScope = (typeof CAPTCHA_SCOPE_ORDER)[number];

const CAPTCHA_PATCH_KEYS: Record<keyof CaptchaSettingsValues, string> = {
  captchaEnabled: "captcha_enabled",
  captchaProvider: "captcha_provider",
  captchaScopes: "captcha_scopes",
  turnstileSiteKey: "turnstile_site_key",
  turnstileSecretKey: "turnstile_secret_key",
};

export function parseCaptchaScopes(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((scope) => scope.trim())
    .filter((scope) => scope !== "");
}

export function serializeCaptchaScopes(scopes: readonly string[]): string {
  const known = CAPTCHA_SCOPE_ORDER.filter((scope) => scopes.includes(scope));
  const unknown = scopes.filter(
    (scope) => !CAPTCHA_SCOPE_ORDER.includes(scope as CaptchaScope),
  );

  return [...known, ...unknown].join(",");
}

export function parseCaptchaSettingsValues(
  payload: unknown,
): CaptchaSettingsValues | null {
  const values = (payload as { values?: unknown } | null)?.values;

  if (typeof values !== "object" || values === null) {
    return null;
  }

  const raw = values as Record<string, unknown>;

  if (
    typeof raw.captcha_enabled !== "boolean" ||
    typeof raw.captcha_provider !== "string" ||
    typeof raw.captcha_scopes !== "string" ||
    typeof raw.turnstile_site_key !== "string" ||
    typeof raw.turnstile_secret_key !== "string"
  ) {
    return null;
  }

  return {
    captchaEnabled: raw.captcha_enabled,
    captchaProvider: raw.captcha_provider,
    captchaScopes: raw.captcha_scopes,
    turnstileSiteKey: raw.turnstile_site_key,
    turnstileSecretKey: raw.turnstile_secret_key,
  };
}

export function hasCaptchaSettingsChanges(
  values: CaptchaSettingsValues,
  baseline: CaptchaSettingsValues,
): boolean {
  const fields = Object.keys(
    CAPTCHA_PATCH_KEYS,
  ) as (keyof CaptchaSettingsValues)[];

  return fields.some((field) => values[field] !== baseline[field]);
}

export function buildCaptchaSettingsPatch(
  values: CaptchaSettingsValues,
  baseline: CaptchaSettingsValues,
): Record<string, string | number | boolean> {
  const patch: Record<string, string | number | boolean> = {};

  if (values.captchaEnabled !== baseline.captchaEnabled) {
    patch[CAPTCHA_PATCH_KEYS.captchaEnabled] = values.captchaEnabled;
  }

  patch[CAPTCHA_PATCH_KEYS.captchaProvider] = values.captchaProvider;
  patch[CAPTCHA_PATCH_KEYS.captchaScopes] = values.captchaScopes;
  patch[CAPTCHA_PATCH_KEYS.turnstileSiteKey] = values.turnstileSiteKey;
  patch[CAPTCHA_PATCH_KEYS.turnstileSecretKey] = values.turnstileSecretKey;

  return patch;
}
