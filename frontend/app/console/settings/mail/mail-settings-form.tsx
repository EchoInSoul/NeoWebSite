"use client";

import { type FormEvent } from "react";
import {
  AtSign,
  Hash,
  KeyRound,
  Lock,
  MailCheck,
  MailWarning,
  Power,
  Send,
  Server,
  Tag,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useMailSettingsSave } from "@/hooks/use-mail-settings-save";
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";
import {
  MAIL_ENCRYPTION_SSL,
  MAIL_ENCRYPTION_STARTTLS,
  MAIL_ENCRYPTIONS,
  type MailSettingsValues,
} from "@/lib/mail-settings";

const MAIL_TEMPLATE_PLACEHOLDERS = [
  "code",
  "expiresMinutes",
  "siteTitle",
  "siteUrl",
  "username",
  "sentAt",
] as const;

const controlClassName = "w-70";

export function MailSettingsForm({
  settings,
}: {
  settings: MailSettingsValues;
}) {
  const t = useTranslations("settings");
  const { values, isDirty, isSaving, update, save } =
    useMailSettingsSave(settings);
  const smtpExpansion = useSwitchRowExpansion(values.enabled);

  const unknownEncryption = MAIL_ENCRYPTIONS.includes(values.smtpEncryption)
    ? null
    : values.smtpEncryption;
  const encryptionItems = {
    [MAIL_ENCRYPTION_SSL]: t("smtpEncryptionSsl"),
    [MAIL_ENCRYPTION_STARTTLS]: t("smtpEncryptionStarttls"),
    ...(unknownEncryption === null
      ? {}
      : { [unknownEncryption]: unknownEncryption }),
  };

  const templateRows = [
    {
      key: "signupCode",
      icon: <MailCheck />,
      label: t("mailTemplateSignupCode"),
      subjectField: "signupCodeSubject",
      bodyField: "signupCodeBody",
    },
    {
      key: "passwordReset",
      icon: <MailWarning />,
      label: t("mailTemplatePasswordReset"),
      subjectField: "passwordResetSubject",
      bodyField: "passwordResetBody",
    },
  ] as const;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
      <ConsoleSettingsGroup title={t("smtpServerGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("smtpEnabled")}
              checked={values.enabled}
              onCheckedChange={(checked) => {
                update("enabled", checked);
                smtpExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expandLabel={t("smtpExpandSettings")}
          expanded={smtpExpansion.expanded}
          expandedContent={
            <>
              <ConsoleSettingsRow
                control={
                  <Input
                    aria-label={t("smtpHost")}
                    autoComplete="off"
                    className={controlClassName}
                    onChange={(event) => update("smtpHost", event.target.value)}
                    spellCheck={false}
                    value={values.smtpHost}
                    disabled={isSaving}
                  />
                }
                icon={<Server />}
                label={t("smtpHost")}
              />
              <ConsoleSettingsRow
                control={
                  <Input
                    aria-label={t("smtpPort")}
                    autoComplete="off"
                    className={controlClassName}
                    inputMode="numeric"
                    onChange={(event) =>
                      update("smtpPort", event.target.value.replace(/\D/g, ""))
                    }
                    spellCheck={false}
                    value={values.smtpPort}
                    disabled={isSaving}
                  />
                }
                icon={<Hash />}
                label={t("smtpPort")}
              />
              <ConsoleSettingsRow
                control={
                  <Select
                    items={encryptionItems}
                    onValueChange={(value) => {
                      if (value !== null) {
                        update("smtpEncryption", value);
                      }
                    }}
                    value={values.smtpEncryption}
                    disabled={isSaving}
                  >
                    <SelectTrigger
                      aria-label={t("smtpEncryption")}
                      className={controlClassName}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={MAIL_ENCRYPTION_SSL}>
                        {t("smtpEncryptionSsl")}
                      </SelectItem>
                      <SelectItem value={MAIL_ENCRYPTION_STARTTLS}>
                        {t("smtpEncryptionStarttls")}
                      </SelectItem>
                      {unknownEncryption === null ? null : (
                        <SelectItem value={unknownEncryption}>
                          {unknownEncryption}
                        </SelectItem>
                      )}
                    </SelectContent>
                  </Select>
                }
                icon={<Lock />}
                label={t("smtpEncryption")}
              />
            </>
          }
          icon={<Power />}
          label={t("smtpEnabled")}
          onExpandedChange={smtpExpansion.setExpanded}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("smtpCredentialsGroup")}>
        <ConsoleSettingsRow
          control={
            <Input
              aria-label={t("smtpUser")}
              autoComplete="off"
              className={controlClassName}
              onChange={(event) => update("smtpUsername", event.target.value)}
              spellCheck={false}
              value={values.smtpUsername}
              disabled={isSaving}
            />
          }
          icon={<AtSign />}
          label={t("smtpUser")}
        />
        <ConsoleSettingsRow
          control={
            <Input
              aria-label={t("smtpPassword")}
              autoComplete="new-password"
              className={controlClassName}
              onChange={(event) => update("smtpPassword", event.target.value)}
              spellCheck={false}
              type="password"
              value={values.smtpPassword}
              disabled={isSaving}
            />
          }
          icon={<KeyRound />}
          label={t("smtpPassword")}
        />
        <ConsoleSettingsRow
          control={
            <Input
              aria-label={t("senderAddress")}
              autoComplete="off"
              className={controlClassName}
              onChange={(event) => update("senderAddress", event.target.value)}
              spellCheck={false}
              value={values.senderAddress}
              disabled={isSaving}
            />
          }
          icon={<Send />}
          label={t("senderAddress")}
        />
        <ConsoleSettingsRow
          control={
            <Input
              aria-label={t("senderName")}
              autoComplete="off"
              className={controlClassName}
              onChange={(event) => update("senderName", event.target.value)}
              spellCheck={false}
              value={values.senderName}
              disabled={isSaving}
            />
          }
          icon={<Tag />}
          label={t("senderName")}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("mailTemplatesGroup")}>
        {templateRows.map((template) => (
          <ConsoleSettingsRow
            key={template.key}
            defaultCollapsed
            expandedContent={
              <div className="flex w-full flex-col gap-4 px-5 py-4">
                <div className="flex flex-col gap-1.5">
                  <label
                    className="text-sm font-medium"
                    htmlFor={`${template.key}.subject`}
                  >
                    {t("mailTemplateSubject")}
                  </label>
                  <Input
                    id={`${template.key}.subject`}
                    autoComplete="off"
                    className="w-full"
                    onChange={(event) =>
                      update(template.subjectField, event.target.value)
                    }
                    spellCheck={false}
                    value={values[template.subjectField]}
                    disabled={isSaving}
                  />
                </div>
                <div className="flex flex-col gap-1.5">
                  <label
                    className="text-sm font-medium"
                    htmlFor={`${template.key}.body`}
                  >
                    {t("mailTemplateBody")}
                  </label>
                  <Textarea
                    id={`${template.key}.body`}
                    aria-describedby={`${template.key}.placeholder-hint`}
                    className="min-h-[320px] resize-y font-mono text-[13px] leading-relaxed"
                    onChange={(event) =>
                      update(template.bodyField, event.target.value)
                    }
                    rows={12}
                    spellCheck={false}
                    value={values[template.bodyField]}
                    disabled={isSaving}
                  />
                </div>
                <div
                  id={`${template.key}.placeholder-hint`}
                  className="rounded-lg border border-input bg-muted/50 px-3 py-2"
                >
                  <p className="text-sm text-muted-foreground">
                    {t("mailTemplatePlaceholderHint")}
                  </p>
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {MAIL_TEMPLATE_PLACEHOLDERS.map((placeholder) => (
                      <code
                        key={placeholder}
                        className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground"
                      >
                        {`{{${placeholder}}}`}
                      </code>
                    ))}
                  </div>
                </div>
              </div>
            }
            icon={template.icon}
            label={template.label}
          />
        ))}
      </ConsoleSettingsGroup>
      <ConsoleFloatingSaveButton
        isDirty={isDirty}
        isPending={isSaving}
        label={t("save")}
      />
    </form>
  );
}
