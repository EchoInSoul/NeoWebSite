import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import type { SessionUser } from "@/lib/auth-session";

export function useProfileSave(profile: SessionUser | null) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [nickname, setNickname] = useState(profile?.nickname ?? "");
  const [email, setEmail] = useState(profile?.email ?? "");
  const [isSaving, setIsSaving] = useState(false);

  const isDirty =
    profile !== null &&
    (nickname !== profile.nickname || email !== profile.email);

  function toFailureMessage(error: unknown): string {
    if (!(error instanceof ApiError)) {
      return t("saveFailed");
    }

    switch (error.code) {
      case "nickname_taken":
        return t("nameTaken");
      case "email_taken":
        return t("emailTaken");
      case "already_taken":
        return t("alreadyTaken");
      case "invalid_request":
        if (error.param === "email") {
          return t("invalidEmail");
        }
        if (error.param === "nickname") {
          return t("invalidNickname");
        }
        return t("saveFailed");
      default:
        return t("saveFailed");
    }
  }

  async function save() {
    if (profile === null || isSaving) {
      return;
    }

    const body: { nickname?: string; email?: string } = {};
    if (nickname !== profile.nickname) {
      body.nickname = nickname;
    }
    if (email !== profile.email) {
      body.email = email;
    }
    if (body.nickname === undefined && body.email === undefined) {
      return;
    }

    setIsSaving(true);
    try {
      await apiRequest("/api/v1/users/me", { method: "PATCH", body });
      toast.success(t("saved"));
      router.refresh();
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        router.replace("/signin");
        return;
      }
      if (looksLikeServiceFailure(error)) {
        toast.error(tCommon("serviceUnavailable"));
        return;
      }
      toast.error(toFailureMessage(error));
    } finally {
      setIsSaving(false);
    }
  }

  return {
    nickname,
    email,
    isDirty,
    isSaving,
    changeNickname: setNickname,
    changeEmail: setEmail,
    save,
  };
}
