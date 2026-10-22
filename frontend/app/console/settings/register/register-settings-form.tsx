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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useRegisterSettingsSave } from "@/hooks/use-register-settings-save";
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";
import type { AdminGroup } from "@/lib/groups";
import {
  groupSelectionMap,
  selectableGroups,
  type RegisterSettingsValues,
} from "@/lib/register-settings";

const inputClassName = "w-70";

export function RegisterSettingsForm({
  settings,
  groups,
}: {
  settings: RegisterSettingsValues;
  groups: AdminGroup[];
}) {
  const t = useTranslations("settings");
  const {
    values,
    isDirty,
    isSaving,
    setEnabled,
    setEmailDomainControlEnabled,
    updateAllowedEmailDomains,
    setDefaultGroup,
    setPasswordRequirementsEnabled,
    updatePasswordMinLength,
    updatePasswordComplexity,
    save,
  } = useRegisterSettingsSave(settings);

  const registrationExpansion = useSwitchRowExpansion(values.enabled);
  const emailDomainExpansion = useSwitchRowExpansion(
    values.emailDomainControlEnabled,
  );
  const passwordRequirementsExpansion = useSwitchRowExpansion(
    values.passwordRequirementsEnabled,
  );

  const defaultGroupOptions = selectableGroups(groups);
  const defaultGroupItems = groupSelectionMap(defaultGroupOptions);
  const defaultGroupKnown = Object.hasOwn(
    defaultGroupItems,
    values.defaultGroup,
  );

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
              onCheckedChange={(checked) => {
                setEnabled(checked);
                registrationExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={registrationExpansion.expanded}
          expandedContent={
            <ConsoleSettingsRow
              control={
                <Select
                  items={defaultGroupItems}
                  value={defaultGroupKnown ? values.defaultGroup : null}
                  onValueChange={setDefaultGroup}
                  disabled={isSaving}
                >
                  <SelectTrigger
                    aria-label={t("registerDefaultGroup")}
                    className="w-45"
                    size="sm"
                  >
                    <SelectValue placeholder={t("registerDefaultGroupNone")} />
                  </SelectTrigger>
                  <SelectContent>
                    {defaultGroupOptions.map((group) => (
                      <SelectItem key={group.id} value={group.name}>
                        {group.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              }
              icon={<UsersRound />}
              label={t("registerDefaultGroup")}
            />
          }
          icon={<Unlock />}
          label={t("allowPublicRegistration")}
          onExpandedChange={registrationExpansion.setExpanded}
        />
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("emailDomainControl")}
              checked={values.emailDomainControlEnabled}
              onCheckedChange={(checked) => {
                setEmailDomainControlEnabled(checked);
                emailDomainExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={emailDomainExpansion.expanded}
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
          onExpandedChange={emailDomainExpansion.setExpanded}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("passwordGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("passwordRequirements")}
              checked={values.passwordRequirementsEnabled}
              onCheckedChange={(checked) => {
                setPasswordRequirementsEnabled(checked);
                passwordRequirementsExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={passwordRequirementsExpansion.expanded}
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
          onExpandedChange={passwordRequirementsExpansion.setExpanded}
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
