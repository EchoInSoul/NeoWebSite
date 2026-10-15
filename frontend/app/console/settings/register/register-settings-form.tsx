"use client";

import { useState } from "react";
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

const inputClassName = "w-70";

export function RegisterSettingsForm() {
  const t = useTranslations("settings");
  const [allowPublicRegistration, setAllowPublicRegistration] = useState(true);
  const [emailDomainControl, setEmailDomainControl] = useState(false);
  const [allowedEmailDomains, setAllowedEmailDomains] = useState("");
  const [passwordRequirements, setPasswordRequirements] = useState(false);
  const [passwordMinLength, setPasswordMinLength] = useState("");
  const [passwordComplexity, setPasswordComplexity] = useState("");

  return (
    <>
      <ConsoleSettingsGroup title={t("registerGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("allowPublicRegistration")}
              checked={allowPublicRegistration}
              onCheckedChange={setAllowPublicRegistration}
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
              checked={emailDomainControl}
              onCheckedChange={setEmailDomainControl}
            />
          }
          expandedContent={
            <ConsoleSettingsRow
              control={
                <Input
                  aria-label={t("allowedEmailDomains")}
                  className={inputClassName}
                  onChange={(event) =>
                    setAllowedEmailDomains(event.target.value)
                  }
                  value={allowedEmailDomains}
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
              checked={passwordRequirements}
              onCheckedChange={setPasswordRequirements}
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
                      setPasswordMinLength(event.target.value)
                    }
                    type="number"
                    value={passwordMinLength}
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
                      setPasswordComplexity(event.target.value)
                    }
                    value={passwordComplexity}
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
    </>
  );
}
