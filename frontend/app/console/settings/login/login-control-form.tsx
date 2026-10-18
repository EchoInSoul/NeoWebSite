"use client";

import { useState } from "react";
import {
  Clock,
  Clock3,
  Fingerprint,
  KeyRound,
  ShieldCheck,
  Smartphone,
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

const SESSION_UNIT_DAY = "day";
const CAPTCHA_PROVIDER_BUILTIN = "builtin";

export function LoginControlForm() {
  const t = useTranslations("settings");
  const [sessionMaxAgeEnabled, setSessionMaxAgeEnabled] = useState(true);
  const [sessionMaxAgeDays, setSessionMaxAgeDays] = useState("7");
  const [sessionMaxAgeUnit, setSessionMaxAgeUnit] = useState(SESSION_UNIT_DAY);
  const [totpEnabled, setTotpEnabled] = useState(true);
  const [passwordResetEnabled, setPasswordResetEnabled] = useState(true);
  const [captchaEnabled, setCaptchaEnabled] = useState(true);
  const [captchaProvider, setCaptchaProvider] = useState(
    CAPTCHA_PROVIDER_BUILTIN,
  );

  const sessionUnitItems = {
    [SESSION_UNIT_DAY]: t("sessionMaxAgeUnitDay"),
  };
  const captchaProviderItems = {
    [CAPTCHA_PROVIDER_BUILTIN]: t("captchaProviderBuiltin"),
  };

  return (
    <div className="flex flex-col gap-4">
      <ConsoleSettingsGroup title={t("sessionGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("sessionMaxAgeEnabled")}
              checked={sessionMaxAgeEnabled}
              onCheckedChange={setSessionMaxAgeEnabled}
            />
          }
          expandedContent={
            <ConsoleSettingsRow
              control={
                <div className="flex items-center gap-2">
                  <Input
                    aria-label={t("sessionMaxAge")}
                    className="w-28"
                    onChange={(event) =>
                      setSessionMaxAgeDays(event.target.value)
                    }
                    type="number"
                    value={sessionMaxAgeDays}
                  />
                  <Select
                    items={sessionUnitItems}
                    value={sessionMaxAgeUnit}
                    onValueChange={(value) => {
                      if (value !== null) {
                        setSessionMaxAgeUnit(value);
                      }
                    }}
                  >
                    <SelectTrigger
                      aria-label={t("sessionMaxAgeUnitDay")}
                      className="w-24"
                      size="sm"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="min-w-0">
                      <SelectItem value={SESSION_UNIT_DAY}>
                        {t("sessionMaxAgeUnitDay")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              }
              icon={<Clock3 />}
              label={t("sessionMaxAge")}
            />
          }
          icon={<Clock />}
          label={t("sessionMaxAgeEnabled")}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("identityGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("twoFactorAuth")}
              checked={totpEnabled}
              onCheckedChange={setTotpEnabled}
            />
          }
          icon={<Smartphone />}
          label={t("twoFactorAuth")}
        />
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("passwordReset")}
              checked={passwordResetEnabled}
              onCheckedChange={setPasswordResetEnabled}
            />
          }
          icon={<KeyRound />}
          label={t("passwordReset")}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("captchaGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("captchaEnabled")}
              checked={captchaEnabled}
              onCheckedChange={setCaptchaEnabled}
            />
          }
          expandedContent={
            <ConsoleSettingsRow
              control={
                <Select
                  items={captchaProviderItems}
                  value={captchaProvider}
                  onValueChange={(value) => {
                    if (value !== null) {
                      setCaptchaProvider(value);
                    }
                  }}
                >
                  <SelectTrigger
                    aria-label={t("captchaProvider")}
                    className="w-45"
                    size="sm"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={CAPTCHA_PROVIDER_BUILTIN}>
                      {t("captchaProviderBuiltin")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              }
              icon={<ShieldCheck />}
              label={t("captchaProvider")}
            />
          }
          icon={<Fingerprint />}
          label={t("captchaEnabled")}
        />
      </ConsoleSettingsGroup>
    </div>
  );
}
