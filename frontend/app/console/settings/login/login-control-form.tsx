"use client";

import { type FormEvent } from "react";
import {
  CalendarClock,
  Fingerprint,
  Globe,
  Hourglass,
  KeyRound,
  LockKeyhole,
  ShieldCheck,
  ShieldUser,
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
import { useLoginSettingsSave } from "@/hooks/use-login-settings-save";
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";
import {
  CAPTCHA_PROVIDER_BUILTIN,
  CAPTCHA_PROVIDER_TURNSTILE,
  SESSION_MAX_AGE_DAYS_MAX,
  SESSION_MAX_AGE_DAYS_MIN,
  type LoginSettingsValues,
} from "@/lib/login-settings";

const KNOWN_CAPTCHA_PROVIDERS: string[] = [
  CAPTCHA_PROVIDER_BUILTIN,
  CAPTCHA_PROVIDER_TURNSTILE,
];

export function LoginControlForm({
  settings,
}: {
  settings: LoginSettingsValues;
}) {
  const t = useTranslations("settings");
  const {
    values,
    isDirty,
    isSaving,
    setSessionMaxAgeEnabled,
    updateSessionMaxAgeDays,
    setTotpEnabled,
    setPasswordResetEnabled,
    setCaptchaEnabled,
    setCaptchaProvider,
    setTurnstileSiteKey,
    setTurnstileSecretKey,
    save,
  } = useLoginSettingsSave(settings);
  const sessionMaxAgeExpansion = useSwitchRowExpansion(
    values.sessionMaxAgeEnabled,
  );
  const captchaExpansion = useSwitchRowExpansion(values.captchaEnabled);

  const unknownCaptchaProvider = KNOWN_CAPTCHA_PROVIDERS.includes(
    values.captchaProvider,
  )
    ? null
    : values.captchaProvider;
  const captchaProviderItems = {
    [CAPTCHA_PROVIDER_BUILTIN]: t("captchaProviderBuiltin"),
    [CAPTCHA_PROVIDER_TURNSTILE]: t("captchaProviderTurnstile"),
    ...(unknownCaptchaProvider === null
      ? {}
      : { [unknownCaptchaProvider]: unknownCaptchaProvider }),
  };

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
      <ConsoleSettingsGroup title={t("sessionGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("sessionMaxAgeEnabled")}
              checked={values.sessionMaxAgeEnabled}
              onCheckedChange={(checked) => {
                setSessionMaxAgeEnabled(checked);
                sessionMaxAgeExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={sessionMaxAgeExpansion.expanded}
          expandedContent={
            <ConsoleSettingsRow
              control={
                <div className="flex items-center gap-2">
                  <Input
                    aria-label={t("sessionMaxAge")}
                    className="w-28"
                    max={SESSION_MAX_AGE_DAYS_MAX}
                    min={SESSION_MAX_AGE_DAYS_MIN}
                    onChange={(event) =>
                      updateSessionMaxAgeDays(event.target.value)
                    }
                    type="number"
                    value={values.sessionMaxAgeDays}
                    disabled={isSaving}
                  />
                  <span className="text-sm text-muted-foreground">
                    {t("sessionMaxAgeUnitDay")}
                  </span>
                </div>
              }
              icon={<CalendarClock />}
              label={t("sessionMaxAge")}
            />
          }
          icon={<Hourglass />}
          label={t("sessionMaxAgeEnabled")}
          onExpandedChange={sessionMaxAgeExpansion.setExpanded}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("identityGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("twoFactorAuth")}
              checked={values.totpEnabled}
              onCheckedChange={setTotpEnabled}
              disabled={isSaving}
            />
          }
          icon={<ShieldUser />}
          label={t("twoFactorAuth")}
        />
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("passwordReset")}
              checked={values.passwordResetEnabled}
              onCheckedChange={setPasswordResetEnabled}
              disabled={isSaving}
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
              checked={values.captchaEnabled}
              onCheckedChange={(checked) => {
                setCaptchaEnabled(checked);
                captchaExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={captchaExpansion.expanded}
          expandedContent={
            <>
              <ConsoleSettingsRow
                control={
                  <Select
                    items={captchaProviderItems}
                    value={values.captchaProvider}
                    onValueChange={(value) => {
                      if (value !== null) {
                        setCaptchaProvider(value);
                      }
                    }}
                    disabled={isSaving}
                  >
                    <SelectTrigger
                      aria-label={t("captchaProvider")}
                      className="w-50"
                      size="sm"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={CAPTCHA_PROVIDER_BUILTIN}>
                        {t("captchaProviderBuiltin")}
                      </SelectItem>
                      <SelectItem value={CAPTCHA_PROVIDER_TURNSTILE}>
                        {t("captchaProviderTurnstile")}
                      </SelectItem>
                      {unknownCaptchaProvider === null ? null : (
                        <SelectItem value={unknownCaptchaProvider}>
                          {unknownCaptchaProvider}
                        </SelectItem>
                      )}
                    </SelectContent>
                  </Select>
                }
                icon={<ShieldCheck />}
                label={t("captchaProvider")}
              />
              {values.captchaProvider === CAPTCHA_PROVIDER_TURNSTILE ? (
                <>
                  <ConsoleSettingsRow
                    control={
                      <Input
                        aria-label={t("captchaTurnstileSiteKey")}
                        autoComplete="off"
                        className="w-64"
                        onChange={(event) =>
                          setTurnstileSiteKey(event.target.value)
                        }
                        spellCheck={false}
                        value={values.turnstileSiteKey}
                        disabled={isSaving}
                      />
                    }
                    icon={<Globe />}
                    label={t("captchaTurnstileSiteKey")}
                  />
                  <ConsoleSettingsRow
                    control={
                      <Input
                        aria-label={t("captchaTurnstileSecretKey")}
                        autoComplete="new-password"
                        className="w-64"
                        onChange={(event) =>
                          setTurnstileSecretKey(event.target.value)
                        }
                        spellCheck={false}
                        type="password"
                        value={values.turnstileSecretKey}
                        disabled={isSaving}
                      />
                    }
                    icon={<LockKeyhole />}
                    label={t("captchaTurnstileSecretKey")}
                  />
                </>
              ) : null}
            </>
          }
          icon={<Fingerprint />}
          label={t("captchaEnabled")}
          onExpandedChange={captchaExpansion.setExpanded}
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
