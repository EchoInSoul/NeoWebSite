"use client";

import type { FormEvent } from "react";
import {
  Asterisk,
  AtSign,
  Hash,
  KeyRound,
  Mail,
  Unlock,
  UsersRound,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useRegisterSettingsSave } from "@/hooks/use-register-settings-save";
import type { RegisterSettingsValues } from "@/lib/register-settings";

const inputClassName = "w-70";

export function RegisterSettingsForm({
  settings,
}: {
  settings: RegisterSettingsValues;
}) {
  const t = useTranslations("settings");
  const {
    values,
    isDirty,
    isSaving,
    setEnabled,
    setEmailDomainControlEnabled,
    updateAllowedEmailDomains,
    setPasswordRequirementsEnabled,
    updatePasswordMinLength,
    updatePasswordComplexity,
    save,
  } = useRegisterSettingsSave(settings);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
      <ConsoleSettingsGroup title={t("registerGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("allowPublicRegistration")}
              checked={values.enabled}
              onCheckedChange={setEnabled}
              disabled={isSaving}
            />
          }
          expandedContent={
            <ConsoleSettingsRow
              control={
                <Select>
                  <SelectTrigger
                    aria-label={t("registerDefaultGroup")}
                    className="w-45"
                    size="sm"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent />
                </Select>
              }
              icon={<UsersRound />}
              label={t("registerDefaultGroup")}
            />
          }
          icon={<Unlock />}
          label={t("allowPublicRegistration")}
        />
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("emailDomainControl")}
              checked={values.emailDomainControlEnabled}
              onCheckedChange={setEmailDomainControlEnabled}
              disabled={isSaving}
            />
          }
          expandedContent={
            <ConsoleSettingsRow
              control={
                <Input
                  aria-label={t("allowedEmailDomains")}
                  className={inputClassName}
                  onChange={(event) =>
                    updateAllowedEmailDomains(event.target.value)
                  }
                  disabled={isSaving}
                  placeholder={
                    values.emailDomainControlEnabled
                      ? t("allowedEmailDomainsEmptyHint")
                      : undefined
                  }
                  value={values.allowedEmailDomains}
                />
              }
              icon={<AtSign />}
              label={t("allowedEmailDomains")}
            />
          }
          icon={<Mail />}
          label={t("emailDomainControl")}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("passwordGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("passwordRequirements")}
              checked={values.passwordRequirementsEnabled}
              onCheckedChange={setPasswordRequirementsEnabled}
              disabled={isSaving}
            />
          }
          expandedContent={
            <>
              <ConsoleSettingsRow
                control={
                  <Input
                    aria-label={t("passwordMinLength")}
                    className={inputClassName}
                    onChange={(event) =>
                      updatePasswordMinLength(event.target.value)
                    }
                    disabled={isSaving}
                    type="number"
                    value={values.passwordMinLength}
                  />
                }
                icon={<Hash />}
                label={t("passwordMinLength")}
              />
              <ConsoleSettingsRow
                control={
                  <Input
                    aria-label={t("passwordComplexity")}
                    className={inputClassName}
                    onChange={(event) =>
                      updatePasswordComplexity(event.target.value)
                    }
                    disabled={isSaving}
                    value={values.passwordComplexity}
                  />
                }
                icon={<Asterisk />}
                label={t("passwordComplexity")}
              />
            </>
          }
          icon={<KeyRound />}
          label={t("passwordRequirements")}
        />
      </ConsoleSettingsGroup>
      <ConsoleFloatingSaveButton
        isDirty={isDirty}
        isPending={isSaving}
        label={t("save")}
      />
    </form>
  );
}
