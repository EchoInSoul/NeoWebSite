export type MailSettingsValues = {
  enabled: boolean;
  smtpHost: string;
  smtpPort: string;
  smtpEncryption: string;
  smtpUsername: string;
  smtpPassword: string;
  senderAddress: string;
  senderName: string;
  signupCodeSubject: string;
  signupCodeBody: string;
  passwordResetSubject: string;
  passwordResetBody: string;
};

export type MailSettingsIssue =
  | "smtpPortRange"
  | "smtpEncryptionInvalid"
  | "smtpPasswordRequired"
  | "senderAddressInvalid";

export const MAIL_ENCRYPTION_SSL = "ssl";
export const MAIL_ENCRYPTION_STARTTLS = "starttls";

export const MAIL_ENCRYPTIONS: string[] = [
  MAIL_ENCRYPTION_SSL,
  MAIL_ENCRYPTION_STARTTLS,
];

export const SMTP_PORT_MIN = 1;
export const SMTP_PORT_MAX = 65535;

const MAIL_SETTINGS_PATCH_KEYS: Record<keyof MailSettingsValues, string> = {
  enabled: "enabled",
  smtpHost: "smtp_host",
  smtpPort: "smtp_port",
  smtpEncryption: "smtp_encryption",
  smtpUsername: "smtp_username",
  smtpPassword: "smtp_password",
  senderAddress: "sender_address",
  senderName: "sender_name",
  signupCodeSubject: "signup_code_subject",
  signupCodeBody: "signup_code_body",
  passwordResetSubject: "password_reset_subject",
  passwordResetBody: "password_reset_body",
};

export function parseMailSettingsValues(
  payload: unknown,
): MailSettingsValues | null {
  const values = (payload as { values?: unknown } | null)?.values;

  if (typeof values !== "object" || values === null) {
    return null;
  }

  const raw = values as Record<string, unknown>;

  if (
    typeof raw.enabled !== "boolean" ||
    typeof raw.smtp_host !== "string" ||
    typeof raw.smtp_port !== "number" ||
    !Number.isInteger(raw.smtp_port) ||
    typeof raw.smtp_encryption !== "string" ||
    typeof raw.smtp_username !== "string" ||
    typeof raw.smtp_password !== "string" ||
    typeof raw.sender_address !== "string" ||
    typeof raw.sender_name !== "string" ||
    typeof raw.signup_code_subject !== "string" ||
    typeof raw.signup_code_body !== "string" ||
    typeof raw.password_reset_subject !== "string" ||
    typeof raw.password_reset_body !== "string"
  ) {
    return null;
  }

  return {
    enabled: raw.enabled,
    smtpHost: raw.smtp_host,
    smtpPort: String(raw.smtp_port),
    smtpEncryption: raw.smtp_encryption,
    smtpUsername: raw.smtp_username,
    smtpPassword: raw.smtp_password,
    senderAddress: raw.sender_address,
    senderName: raw.sender_name,
    signupCodeSubject: raw.signup_code_subject,
    signupCodeBody: raw.signup_code_body,
    passwordResetSubject: raw.password_reset_subject,
    passwordResetBody: raw.password_reset_body,
  };
}

export function hasMailSettingsChanges(
  values: MailSettingsValues,
  baseline: MailSettingsValues,
): boolean {
  const fields = Object.keys(
    MAIL_SETTINGS_PATCH_KEYS,
  ) as (keyof MailSettingsValues)[];

  return fields.some((field) => values[field] !== baseline[field]);
}

function smtpPortInvalid(raw: string): boolean {
  const parsed = Number(raw);

  return (
    !Number.isInteger(parsed) ||
    parsed < SMTP_PORT_MIN ||
    parsed > SMTP_PORT_MAX
  );
}

function senderAddressValid(raw: string): boolean {
  const trimmed = raw.trim();

  return trimmed === raw && /^[^\s@]+@[^\s@]+$/.test(trimmed);
}

export function validateMailSettings(
  values: MailSettingsValues,
  baseline: MailSettingsValues,
): MailSettingsIssue | null {
  const hostConfigured = values.enabled && values.smtpHost.trim() !== "";
  const portTouched = values.smtpPort !== baseline.smtpPort;
  const encryptionTouched = values.smtpEncryption !== baseline.smtpEncryption;

  if ((portTouched || hostConfigured) && smtpPortInvalid(values.smtpPort)) {
    return "smtpPortRange";
  }

  if (
    (encryptionTouched || hostConfigured) &&
    !MAIL_ENCRYPTIONS.includes(values.smtpEncryption)
  ) {
    return "smtpEncryptionInvalid";
  }

  if (!hostConfigured) {
    return null;
  }

  if (values.smtpUsername.trim() !== "" && values.smtpPassword === "") {
    return "smtpPasswordRequired";
  }

  if (!senderAddressValid(values.senderAddress)) {
    return "senderAddressInvalid";
  }

  return null;
}

export function buildMailSettingsPatch(
  values: MailSettingsValues,
  baseline: MailSettingsValues,
): Record<string, string | number | boolean> {
  const patch: Record<string, string | number | boolean> = {};
  const fields = Object.keys(
    MAIL_SETTINGS_PATCH_KEYS,
  ) as (keyof MailSettingsValues)[];

  for (const field of fields) {
    if (values[field] === baseline[field]) {
      continue;
    }

    if (field === "smtpPort") {
      patch[MAIL_SETTINGS_PATCH_KEYS[field]] = Number(values.smtpPort);
    } else {
      patch[MAIL_SETTINGS_PATCH_KEYS[field]] = values[field];
    }
  }

  return patch;
}
