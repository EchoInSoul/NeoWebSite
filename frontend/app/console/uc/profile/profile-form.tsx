"use client";

import type { FormEvent } from "react";
import { Mail, ShieldCheck, UserRound, UsersRound } from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Input } from "@/components/ui/input";
import { useProfileSave } from "@/hooks/use-profile-save";
import type { SessionUser } from "@/lib/auth-session";

const missingValue = "—";
const valueClassName =
  "min-w-0 max-w-full truncate text-sm text-muted-foreground";
const inputClassName = "w-70";

export function ProfileForm({ profile }: { profile: SessionUser | null }) {
  const t = useTranslations("uc");
  const {
    nickname,
    email,
    isDirty,
    isSaving,
    changeNickname,
    changeEmail,
    save,
  } = useProfileSave(profile);

  const groupDescription =
    profile === null
      ? undefined
      : profile.group.systemKey === null
        ? t("customGroup")
        : t("presetGroup", { key: profile.group.systemKey });

  const twoFactorValue =
    profile === null
      ? missingValue
      : t(profile.twoFactorEnabled ? "twoFactorEnabled" : "twoFactorDisabled");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  return (
    <form onSubmit={handleSubmit}>
      <ConsoleSettingsGroup title={t("account")}>
        <ConsoleSettingsRow
          control={
            <Input
              aria-label={t("username")}
              autoComplete="nickname"
              className={inputClassName}
              disabled={profile === null || isSaving}
              onChange={(event) => changeNickname(event.target.value)}
              value={nickname}
            />
          }
          description={t("usernameHelp")}
          icon={<UserRound />}
          label={t("username")}
        />
        <ConsoleSettingsRow
          control={
            <Input
              aria-label={t("email")}
              autoComplete="email"
              className={inputClassName}
              disabled={profile === null || isSaving}
              onChange={(event) => changeEmail(event.target.value)}
              type="email"
              value={email}
            />
          }
          description={t("emailHelp")}
          icon={<Mail />}
          label={t("email")}
        />
        <ConsoleSettingsRow
          control={
            <span className={valueClassName}>
              {profile?.group.name ?? missingValue}
            </span>
          }
          description={groupDescription}
          icon={<UsersRound />}
          label={t("group")}
        />
        <ConsoleSettingsRow
          control={<span className={valueClassName}>{twoFactorValue}</span>}
          icon={<ShieldCheck />}
          label={t("twoFactor")}
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
