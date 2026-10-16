export type RegisterGroupValues = {
  enabled: boolean;
  emailDomainControlEnabled: boolean;
  allowedEmailDomains: string;
};

export type AuthGroupValues = {
  passwordRequirementsEnabled: boolean;
  passwordMinLength: string;
  passwordComplexity: string;
};

export type RegisterSettingsValues = RegisterGroupValues & AuthGroupValues;

export type RegisterSettingsPatch = {
  register?: Record<string, string | boolean>;
  auth?: Record<string, string | number | boolean>;
};

export type RegisterSettingsIssue = "passwordMinLengthRange";

const REGISTER_PATCH_KEYS: Record<keyof RegisterGroupValues, string> = {
  enabled: "enabled",
  emailDomainControlEnabled: "email_domain_control_enabled",
  allowedEmailDomains: "allowed_email_domains",
};

const AUTH_PATCH_KEYS: Record<keyof AuthGroupValues, string> = {
  passwordRequirementsEnabled: "password_requirements_enabled",
  passwordMinLength: "password_min_length",
  passwordComplexity: "password_complexity",
};

export function parseRegisterGroupValues(
  payload: unknown,
): RegisterGroupValues | null {
  const values = (payload as { values?: unknown } | null)?.values;

  if (typeof values !== "object" || values === null) {
    return null;
  }

  const raw = values as Record<string, unknown>;

  if (
    typeof raw.enabled !== "boolean" ||
    typeof raw.email_domain_control_enabled !== "boolean" ||
    typeof raw.allowed_email_domains !== "string"
  ) {
    return null;
  }

  return {
    enabled: raw.enabled,
    emailDomainControlEnabled: raw.email_domain_control_enabled,
    allowedEmailDomains: raw.allowed_email_domains,
  };
}

export function parseAuthGroupValues(payload: unknown): AuthGroupValues | null {
  const values = (payload as { values?: unknown } | null)?.values;

  if (typeof values !== "object" || values === null) {
    return null;
  }

  const raw = values as Record<string, unknown>;

  if (
    typeof raw.password_requirements_enabled !== "boolean" ||
    typeof raw.password_min_length !== "number" ||
    !Number.isInteger(raw.password_min_length) ||
    typeof raw.password_complexity !== "string"
  ) {
    return null;
  }

  return {
    passwordRequirementsEnabled: raw.password_requirements_enabled,
    passwordMinLength: String(raw.password_min_length),
    passwordComplexity: raw.password_complexity,
  };
}

export function parseRegisterSettingsValues(
  registerPayload: unknown,
  authPayload: unknown,
): RegisterSettingsValues | null {
  const register = parseRegisterGroupValues(registerPayload);
  const auth = parseAuthGroupValues(authPayload);

  if (register === null || auth === null) {
    return null;
  }

  return { ...register, ...auth };
}

export function hasRegisterSettingsChanges(
  values: RegisterSettingsValues,
  baseline: RegisterSettingsValues,
): boolean {
  const registerFields = Object.keys(
    REGISTER_PATCH_KEYS,
  ) as (keyof RegisterGroupValues)[];
  const authFields = Object.keys(AUTH_PATCH_KEYS) as (keyof AuthGroupValues)[];

  for (const field of registerFields) {
    if (values[field] !== baseline[field]) {
      return true;
    }
  }

  for (const field of authFields) {
    if (values[field] !== baseline[field]) {
      return true;
    }
  }

  return false;
}

function passwordMinLengthInvalid(raw: string): boolean {
  const parsed = Number(raw);

  return !Number.isInteger(parsed) || parsed < 8 || parsed > 64;
}

export function validateRegisterSettings(
  values: RegisterSettingsValues,
  baseline: RegisterSettingsValues,
): RegisterSettingsIssue | null {
  if (
    values.passwordMinLength !== baseline.passwordMinLength &&
    passwordMinLengthInvalid(values.passwordMinLength)
  ) {
    return "passwordMinLengthRange";
  }

  return null;
}

export function buildRegisterSettingsPatch(
  values: RegisterSettingsValues,
  baseline: RegisterSettingsValues,
): RegisterSettingsPatch {
  const patch: RegisterSettingsPatch = {};

  const registerFields = Object.keys(
    REGISTER_PATCH_KEYS,
  ) as (keyof RegisterGroupValues)[];
  const registerPatch: Record<string, string | boolean> = {};

  for (const field of registerFields) {
    if (values[field] !== baseline[field]) {
      registerPatch[REGISTER_PATCH_KEYS[field]] = values[field];
    }
  }

  if (Object.keys(registerPatch).length > 0) {
    patch.register = registerPatch;
  }

  const authFields = Object.keys(AUTH_PATCH_KEYS) as (keyof AuthGroupValues)[];
  const authPatch: Record<string, string | number | boolean> = {};

  for (const field of authFields) {
    if (values[field] !== baseline[field]) {
      if (field === "passwordMinLength") {
        authPatch[AUTH_PATCH_KEYS[field]] = Number(values.passwordMinLength);
      } else {
        authPatch[AUTH_PATCH_KEYS[field]] = values[field];
      }
    }
  }

  if (Object.keys(authPatch).length > 0) {
    patch.auth = authPatch;
  }

  return patch;
}
