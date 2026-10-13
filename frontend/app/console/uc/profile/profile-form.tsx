"use client";

import { useState, type FormEvent, type KeyboardEvent } from "react";
import {
  AlignLeft,
  Eye,
  EyeOff,
  KeyRound,
  Mail,
  ShieldCheck,
  UserRound,
  UsersRound,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useProfileSave } from "@/hooks/use-profile-save";
import type { SessionUser } from "@/lib/auth-session";

const missingValue = "—";
const valueClassName =
  "min-w-0 max-w-full truncate text-sm text-muted-foreground";
const inputClassName = "w-70";
const inputGroupClassName = "w-70";
const textareaClassName = "w-70 min-h-20";

export function ProfileForm({ profile }: { profile: SessionUser | null }) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const {
    nickname,
    email,
    bio,
    isDirty,
    isSaving,
    changeNickname,
    changeEmail,
    changeBio,
    save,
  } = useProfileSave(profile);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [isCurrentPasswordVisible, setIsCurrentPasswordVisible] =
    useState(false);
  const [isNewPasswordVisible, setIsNewPasswordVisible] = useState(false);
  const [twoFactorEnabled, setTwoFactorEnabled] = useState(
    profile?.twoFactorEnabled ?? false,
  );

  const isLocked = profile === null || isSaving;

  const groupDescription =
    profile === null
      ? undefined
      : profile.group.systemKey === null
        ? t("customGroup")
        : t("presetGroup", { key: profile.group.systemKey });

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  function ignoreEnter(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
    }
  }

  return (
    <form onSubmit={handleSubmit} noValidate>
      <div className="flex flex-col gap-5">
        <ConsoleSettingsGroup title={t("profile")}>
          <ConsoleSettingsRow
            control={
              <Input
                aria-label={t("username")}
                autoComplete="nickname"
                className={inputClassName}
                disabled={isLocked}
                onChange={(event) => changeNickname(event.target.value)}
                value={nickname}
              />
            }
            description={t("usernameHelp")}
            icon={<UserRound />}
            label={t("username")}
          />
          <ConsoleSettingsRow
            className="items-start"
            control={
              <Textarea
                aria-label={t("bio")}
                className={textareaClassName}
                disabled={isLocked}
                onChange={(event) => changeBio(event.target.value)}
                value={bio}
              />
            }
            description={t("bioHelp")}
            icon={<AlignLeft />}
            label={t("bio")}
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
        </ConsoleSettingsGroup>
        <ConsoleSettingsGroup title={t("account")}>
          <ConsoleSettingsRow
            control={
              <Input
                aria-label={t("email")}
                autoComplete="email"
                className={inputClassName}
                disabled={isLocked}
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
              <Switch
                aria-label={t("twoFactor")}
                checked={twoFactorEnabled}
                disabled={isLocked}
                onCheckedChange={setTwoFactorEnabled}
              />
            }
            icon={<ShieldCheck />}
            label={t("twoFactor")}
          />
          <ConsoleSettingsRow
            description={t("changePassword")}
            expandedContent={
              <>
                <ConsoleSettingsRow
                  control={
                    <InputGroup className={inputGroupClassName}>
                      <InputGroupInput
                        aria-label={t("currentPassword")}
                        autoComplete="current-password"
                        disabled={isLocked}
                        onChange={(event) =>
                          setCurrentPassword(event.target.value)
                        }
                        onKeyDown={ignoreEnter}
                        type={isCurrentPasswordVisible ? "text" : "password"}
                        value={currentPassword}
                      />
                      <InputGroupAddon align="inline-end">
                        <InputGroupButton
                          size="icon-xs"
                          aria-label={
                            isCurrentPasswordVisible
                              ? tCommon("hidePassword")
                              : tCommon("showPassword")
                          }
                          disabled={isLocked}
                          onClick={() =>
                            setIsCurrentPasswordVisible((visible) => !visible)
                          }
                        >
                          {isCurrentPasswordVisible ? <EyeOff /> : <Eye />}
                        </InputGroupButton>
                      </InputGroupAddon>
                    </InputGroup>
                  }
                  label={t("currentPassword")}
                />
                <ConsoleSettingsRow
                  control={
                    <InputGroup className={inputGroupClassName}>
                      <InputGroupInput
                        aria-label={t("newPassword")}
                        autoComplete="new-password"
                        disabled={isLocked}
                        onChange={(event) => setNewPassword(event.target.value)}
                        onKeyDown={ignoreEnter}
                        type={isNewPasswordVisible ? "text" : "password"}
                        value={newPassword}
                      />
                      <InputGroupAddon align="inline-end">
                        <InputGroupButton
                          size="icon-xs"
                          aria-label={
                            isNewPasswordVisible
                              ? tCommon("hidePassword")
                              : tCommon("showPassword")
                          }
                          disabled={isLocked}
                          onClick={() =>
                            setIsNewPasswordVisible((visible) => !visible)
                          }
                        >
                          {isNewPasswordVisible ? <EyeOff /> : <Eye />}
                        </InputGroupButton>
                      </InputGroupAddon>
                    </InputGroup>
                  }
                  label={t("newPassword")}
                />
              </>
            }
            icon={<KeyRound />}
            label={t("password")}
          />
        </ConsoleSettingsGroup>
      </div>
      <ConsoleFloatingSaveButton
        isDirty={isDirty}
        isPending={isSaving}
        label={t("save")}
      />
    </form>
  );
}
