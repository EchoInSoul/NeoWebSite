"use client";

import { useState } from "react";
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
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";

const ENCRYPTION_SSL = "ssl";
const ENCRYPTION_STARTTLS = "starttls";
const ENCRYPTION_NONE = "none";

const MAIL_TEMPLATE_PLACEHOLDERS = [
  "code",
  "expiresMinutes",
  "siteTitle",
  "siteUrl",
  "username",
  "sentAt",
] as const;

type MailTemplateKey = "signupCode" | "passwordReset";

type MailTemplateValue = {
  subject: string;
  body: string;
};

const controlClassName = "w-70";

export function MailSettingsForm() {
  const t = useTranslations("settings");
  const [enabled, setEnabled] = useState(false);
  const [smtpHost, setSmtpHost] = useState("");
  const [smtpPort, setSmtpPort] = useState("465");
  const [encryption, setEncryption] = useState<string>(ENCRYPTION_SSL);
  const [smtpUser, setSmtpUser] = useState("");
  const [smtpPassword, setSmtpPassword] = useState("");
  const [senderAddress, setSenderAddress] = useState("");
  const [senderName, setSenderName] = useState("");
  const [templateValues, setTemplateValues] = useState<
    Record<MailTemplateKey, MailTemplateValue>
  >({
    signupCode: { subject: "", body: "" },
    passwordReset: { subject: "", body: "" },
  });
  const smtpExpansion = useSwitchRowExpansion(enabled);

  function updateTemplateValue(
    templateKey: MailTemplateKey,
    field: keyof MailTemplateValue,
    value: string,
  ) {
    setTemplateValues((previous) => ({
      ...previous,
      [templateKey]: { ...previous[templateKey], [field]: value },
    }));
  }

  const templateRows = [
    {
      key: "signupCode",
      icon: <MailCheck />,
      label: t("mailTemplateSignupCode"),
    },
    {
      key: "passwordReset",
      icon: <MailWarning />,
      label: t("mailTemplatePasswordReset"),
    },
  ] as const;

  const encryptionItems = {
    [ENCRYPTION_SSL]: t("smtpEncryptionSsl"),
    [ENCRYPTION_STARTTLS]: t("smtpEncryptionStarttls"),
    [ENCRYPTION_NONE]: t("smtpEncryptionNone"),
  };

  return (
    <div className="flex flex-col gap-4">
      <ConsoleSettingsGroup title={t("smtpServerGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("smtpEnabled")}
              checked={enabled}
              onCheckedChange={(checked) => {
                setEnabled(checked);
                smtpExpansion.expandOnEnable(checked);
              }}
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
                    onChange={(event) => setSmtpHost(event.target.value)}
                    spellCheck={false}
                    value={smtpHost}
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
                      setSmtpPort(event.target.value.replace(/\D/g, ""))
                    }
                    spellCheck={false}
                    value={smtpPort}
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
                        setEncryption(value);
                      }
                    }}
                    value={encryption}
                  >
                    <SelectTrigger
                      aria-label={t("smtpEncryption")}
                      className={controlClassName}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={ENCRYPTION_SSL}>
                        {t("smtpEncryptionSsl")}
                      </SelectItem>
                      <SelectItem value={ENCRYPTION_STARTTLS}>
                        {t("smtpEncryptionStarttls")}
                      </SelectItem>
                      <SelectItem value={ENCRYPTION_NONE}>
                        {t("smtpEncryptionNone")}
                      </SelectItem>
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
              onChange={(event) => setSmtpUser(event.target.value)}
              spellCheck={false}
              value={smtpUser}
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
              onChange={(event) => setSmtpPassword(event.target.value)}
              spellCheck={false}
              type="password"
              value={smtpPassword}
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
              onChange={(event) => setSenderAddress(event.target.value)}
              spellCheck={false}
              value={senderAddress}
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
              onChange={(event) => setSenderName(event.target.value)}
              spellCheck={false}
              value={senderName}
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
                      updateTemplateValue(
                        template.key,
                        "subject",
                        event.target.value,
                      )
                    }
                    spellCheck={false}
                    value={templateValues[template.key].subject}
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
                      updateTemplateValue(
                        template.key,
                        "body",
                        event.target.value,
                      )
                    }
                    rows={12}
                    spellCheck={false}
                    value={templateValues[template.key].body}
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
    </div>
  );
}
