"use client";

import { useState, type FormEvent, type KeyboardEvent } from "react";
import {
  Eye,
  EyeOff,
  KeyRound,
  Mail,
  ShieldCheck,
  TextInitial,
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
import { usePasswordChange } from "@/hooks/use-password-change";
import { useProfileSave } from "@/hooks/use-profile-save";
import type { SessionUser } from "@/lib/auth-session";

import { TwoFactorDisablePanel } from "./two-factor-disable-panel";
import { TwoFactorPanel } from "./two-factor-panel";

const missingValue = "—";
const valueClassName =
  "min-w-0 max-w-full truncate text-sm text-muted-foreground";
const inputClassName = "w-70";
const inputGroupClassName = "w-70";
const textareaClassName = "w-70 min-h-20";

export function ProfileForm({
  profile,
  totpAvailable,
}: {
  profile: SessionUser | null;
  totpAvailable: boolean;
}) {
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
  const {
    currentPassword,
    newPassword,
    isSubmitting: isChangingPassword,
    changeCurrentPassword,
    changeNewPassword,
    submit: submitPasswordChange,
  } = usePasswordChange();
  const [isCurrentPasswordVisible, setIsCurrentPasswordVisible] =
    useState(false);
  const [isNewPasswordVisible, setIsNewPasswordVisible] = useState(false);
  const twoFactorEnabled = profile?.twoFactorEnabled ?? false;
  const showTwoFactor = totpAvailable || twoFactorEnabled;
  const [isTwoFactorExpanded, setIsTwoFactorExpanded] = useState(false);

  const isLocked = profile === null || isSaving;
  const isPasswordLocked = profile === null || isChangingPassword;
  const isPasswordFilled = currentPassword !== "" && newPassword !== "";

  function handlePasswordKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      void submitPasswordChange();
    }
  }

  function openTwoFactorPanel() {
    setIsTwoFactorExpanded(true);
  }

  function closeTwoFactorPanel() {
    setIsTwoFactorExpanded(false);
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void submitPasswordChange();
    void save();
  }

  return (
    <form method="post" onSubmit={handleSubmit} noValidate>
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
            icon={<TextInitial />}
            label={t("bio")}
          />
          <ConsoleSettingsRow
            control={
              <span className={valueClassName}>
                {profile?.group.name ?? missingValue}
              </span>
            }
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
            icon={<Mail />}
            label={t("email")}
          />
          {showTwoFactor ? (
            <ConsoleSettingsRow
              control={
                <Switch
                  aria-label={t("twoFactor")}
                  checked={twoFactorEnabled}
                  disabled={isLocked}
                  onCheckedChange={openTwoFactorPanel}
                />
              }
              expanded={isTwoFactorExpanded}
              expandedContent={
                twoFactorEnabled ? (
                  <TwoFactorDisablePanel onDone={closeTwoFactorPanel} />
                ) : (
                  <TwoFactorPanel onDone={closeTwoFactorPanel} />
                )
              }
              icon={<ShieldCheck />}
              label={t("twoFactor")}
              onExpandedChange={setIsTwoFactorExpanded}
            />
          ) : null}
          <ConsoleSettingsRow
            defaultCollapsed
            expandedContent={
              <>
                <ConsoleSettingsRow
                  control={
                    <InputGroup className={inputGroupClassName}>
                      <InputGroupInput
                        aria-label={t("currentPassword")}
                        autoComplete="current-password"
                        disabled={isPasswordLocked}
                        onChange={(event) =>
                          changeCurrentPassword(event.target.value)
                        }
                        onKeyDown={handlePasswordKeyDown}
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
                          disabled={isPasswordLocked}
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
                        disabled={isPasswordLocked}
                        onChange={(event) =>
                          changeNewPassword(event.target.value)
                        }
                        onKeyDown={handlePasswordKeyDown}
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
                          disabled={isPasswordLocked}
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
        isDirty={isDirty || isPasswordFilled}
        isPending={isSaving || isChangingPassword}
        label={t("save")}
      />
    </form>
  );
}
