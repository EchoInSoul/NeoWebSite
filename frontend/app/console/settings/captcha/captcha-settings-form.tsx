"use client";

import { type FormEvent, useState } from "react";
import {
  BadgeCheck,
  Fingerprint,
  Globe,
  ListChecks,
  LockKeyhole,
  ShieldCheck,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { CaptchaDialog } from "@/components/auth/captcha-dialog";
import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useCaptchaSettingsSave } from "@/hooks/use-captcha-settings-save";
import { useCaptchaVerify } from "@/hooks/use-captcha-verify";
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";
import {
  CAPTCHA_PROVIDER_BUILTIN,
  CAPTCHA_PROVIDER_NONE,
  CAPTCHA_PROVIDER_TURNSTILE,
  CAPTCHA_SCOPE_LOGIN,
  CAPTCHA_SCOPE_REGISTER,
  parseCaptchaScopes,
  serializeCaptchaScopes,
  type CaptchaSettingsValues,
} from "@/lib/captcha-settings";

const KNOWN_CAPTCHA_PROVIDERS: string[] = [
  CAPTCHA_PROVIDER_NONE,
  CAPTCHA_PROVIDER_BUILTIN,
  CAPTCHA_PROVIDER_TURNSTILE,
];

export function CaptchaSettingsForm({
  settings,
}: {
  settings: CaptchaSettingsValues;
}) {
  const t = useTranslations("settings");
  const {
    values,
    isDirty,
    isSaving,
    setCaptchaEnabled,
    setCaptchaProvider,
    setCaptchaScopes,
    setTurnstileSiteKey,
    setTurnstileSecretKey,
    save,
  } = useCaptchaSettingsSave(settings);
  const captchaExpansion = useSwitchRowExpansion(values.captchaEnabled);
  const { isVerifying, verify } = useCaptchaVerify();
  const [isVerifyOpen, setIsVerifyOpen] = useState(false);

  const captchaScopes = parseCaptchaScopes(values.captchaScopes);
  const captchaScopeItems: Record<string, string> = {
    [CAPTCHA_SCOPE_LOGIN]: t("captchaScopeLogin"),
    [CAPTCHA_SCOPE_REGISTER]: t("captchaScopeRegister"),
  };

  const unknownCaptchaProvider = KNOWN_CAPTCHA_PROVIDERS.includes(
    values.captchaProvider,
  )
    ? null
    : values.captchaProvider;
  const captchaProviderItems = {
    [CAPTCHA_PROVIDER_NONE]: t("captchaProviderNone"),
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

  function handleVerifyClick() {
    if (values.turnstileSiteKey === "" || values.turnstileSecretKey === "") {
      toast.error(t("captchaVerifyMissingKeys"));
      return;
    }
    setIsVerifyOpen(true);
  }

  function handleCaptchaSolved(token: string) {
    setIsVerifyOpen(false);
    void verify(values.turnstileSecretKey, token);
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
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
                      className="w-64"
                      size="sm"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={CAPTCHA_PROVIDER_NONE}>
                        {t("captchaProviderNone")}
                      </SelectItem>
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
              {values.captchaProvider === CAPTCHA_PROVIDER_TURNSTILE ||
              values.captchaProvider === CAPTCHA_PROVIDER_BUILTIN ? (
                <ConsoleSettingsRow
                  control={
                    <Select
                      items={captchaScopeItems}
                      multiple
                      value={captchaScopes}
                      onValueChange={(value) => {
                        setCaptchaScopes(serializeCaptchaScopes(value));
                      }}
                      disabled={isSaving}
                    >
                      <SelectTrigger
                        aria-label={t("captchaScopes")}
                        className="w-64"
                        size="sm"
                      >
                        <SelectValue>
                          {(value: string[]) =>
                            value.length === 0
                              ? t("captchaScopesPlaceholder")
                              : value
                                  .map(
                                    (scope) =>
                                      captchaScopeItems[scope] ?? scope,
                                  )
                                  .join("、")
                          }
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={CAPTCHA_SCOPE_LOGIN}>
                          {t("captchaScopeLogin")}
                        </SelectItem>
                        <SelectItem value={CAPTCHA_SCOPE_REGISTER}>
                          {t("captchaScopeRegister")}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  }
                  icon={<ListChecks />}
                  label={t("captchaScopes")}
                />
              ) : null}
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
                        disabled={isSaving || isVerifying}
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
                        disabled={isSaving || isVerifying}
                      />
                    }
                    icon={<LockKeyhole />}
                    label={t("captchaTurnstileSecretKey")}
                  />
                  <ConsoleSettingsRow
                    control={
                      <Button
                        aria-label={t("captchaVerifyConfig")}
                        disabled={isSaving || isVerifying}
                        onClick={handleVerifyClick}
                        size="sm"
                        type="button"
                        variant="outline"
                      >
                        {t("captchaVerify")}
                      </Button>
                    }
                    icon={<BadgeCheck />}
                    label={t("captchaVerifyConfig")}
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
      <CaptchaDialog
        open={isVerifyOpen}
        config={{ provider: "turnstile", siteKey: values.turnstileSiteKey }}
        onOpenChange={setIsVerifyOpen}
        onSolved={handleCaptchaSolved}
      />
    </form>
  );
}
