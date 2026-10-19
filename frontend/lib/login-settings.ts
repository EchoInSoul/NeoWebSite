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

export const SESSION_MAX_AGE_DAYS_MIN = 1;
export const SESSION_MAX_AGE_DAYS_MAX = 30;

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
    typeof raw.captcha_provider !== "string"
  ) {
    return null;
  }

  return {
    sessionMaxAgeEnabled: raw.session_max_age_enabled,
    sessionMaxAgeDays: String(raw.session_max_age_days),
    totpEnabled: raw.totp_enabled,
    passwordResetEnabled: raw.password_reset_enabled,
    captchaEnabled: raw.captcha_enabled,
    captchaProvider:
      raw.captcha_provider === ""
        ? CAPTCHA_PROVIDER_BUILTIN
        : raw.captcha_provider,
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

  return (
    !Number.isInteger(parsed) ||
    parsed < SESSION_MAX_AGE_DAYS_MIN ||
    parsed > SESSION_MAX_AGE_DAYS_MAX
  );
}

export function validateLoginSettings(
  values: LoginSettingsValues,
  baseline: LoginSettingsValues,
): LoginSettingsIssue | null {
  const daysWritten = values.sessionMaxAgeDays !== baseline.sessionMaxAgeDays;
  const enabledTurnedOn =
    values.sessionMaxAgeEnabled !== baseline.sessionMaxAgeEnabled &&
    values.sessionMaxAgeEnabled;

  if (
    (daysWritten || enabledTurnedOn) &&
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

  patch[LOGIN_PATCH_KEYS.captchaProvider] = values.captchaProvider;

  return patch;
}
