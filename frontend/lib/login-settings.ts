export type LoginSettingsValues = {
  sessionMaxAgeEnabled: boolean;
  sessionMaxAgeDays: string;
  totpEnabled: boolean;
  passwordResetEnabled: boolean;
  captchaEnabled: boolean;
  captchaProvider: string;
};

export type LoginSettingsIssue = "sessionMaxAgeDaysRange";

export const CAPTCHA_PROVIDER_BUILTIN = "builtin";

const LOGIN_PATCH_KEYS: Record<keyof LoginSettingsValues, string> = {
  sessionMaxAgeEnabled: "session_max_age_enabled",
  sessionMaxAgeDays: "session_max_age_days",
  totpEnabled: "totp_enabled",
  passwordResetEnabled: "password_reset_enabled",
  captchaEnabled: "captcha_enabled",
  captchaProvider: "captcha_provider",
};

export function parseLoginGroupValues(
  payload: unknown,
): LoginSettingsValues | null {
  const values = (payload as { values?: unknown } | null)?.values;

  if (typeof values !== "object" || values === null) {
    return null;
  }

  const raw = values as Record<string, unknown>;

  if (
    typeof raw.session_max_age_enabled !== "boolean" ||
    typeof raw.session_max_age_days !== "number" ||
    !Number.isInteger(raw.session_max_age_days) ||
    typeof raw.totp_enabled !== "boolean" ||
    typeof raw.password_reset_enabled !== "boolean" ||
    typeof raw.captcha_enabled !== "boolean" ||
    typeof raw.captcha_provider !== "string" ||
    (raw.captcha_provider !== "" &&
      raw.captcha_provider !== CAPTCHA_PROVIDER_BUILTIN)
  ) {
    return null;
  }

  return {
    sessionMaxAgeEnabled: raw.session_max_age_enabled,
    sessionMaxAgeDays: String(raw.session_max_age_days),
    totpEnabled: raw.totp_enabled,
    passwordResetEnabled: raw.password_reset_enabled,
    captchaEnabled: raw.captcha_enabled,
    captchaProvider: CAPTCHA_PROVIDER_BUILTIN,
  };
}

export function hasLoginSettingsChanges(
  values: LoginSettingsValues,
  baseline: LoginSettingsValues,
): boolean {
  const fields = Object.keys(LOGIN_PATCH_KEYS) as (keyof LoginSettingsValues)[];

  return fields.some((field) => values[field] !== baseline[field]);
}

function sessionMaxAgeDaysInvalid(raw: string): boolean {
  const parsed = Number(raw);

  return !Number.isInteger(parsed) || parsed < 1 || parsed > 30;
}

export function validateLoginSettings(
  values: LoginSettingsValues,
  baseline: LoginSettingsValues,
): LoginSettingsIssue | null {
  if (
    (values.sessionMaxAgeEnabled ||
      values.sessionMaxAgeDays !== baseline.sessionMaxAgeDays) &&
    sessionMaxAgeDaysInvalid(values.sessionMaxAgeDays)
  ) {
    return "sessionMaxAgeDaysRange";
  }

  return null;
}

export function buildLoginSettingsPatch(
  values: LoginSettingsValues,
  baseline: LoginSettingsValues,
): Record<string, string | number | boolean> {
  const patch: Record<string, string | number | boolean> = {};
  const fields = Object.keys(LOGIN_PATCH_KEYS) as (keyof LoginSettingsValues)[];

  for (const field of fields) {
    if (values[field] === baseline[field]) {
      continue;
    }

    if (field === "sessionMaxAgeDays") {
      patch[LOGIN_PATCH_KEYS[field]] = Number(values.sessionMaxAgeDays);
    } else {
      patch[LOGIN_PATCH_KEYS[field]] = values[field];
    }
  }

  return patch;
}
