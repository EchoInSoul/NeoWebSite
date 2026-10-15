import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import type { SessionUser } from "@/lib/auth-session";

type ProfileUpdateResponse = {
  email: string;
  nickname: string;
};

export function useProfileSave(profile: SessionUser | null) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [nickname, setNickname] = useState(profile?.nickname ?? "");
  const [email, setEmail] = useState(profile?.email ?? "");
  const [bio, setBio] = useState("");
  const [savedBio, setSavedBio] = useState("");
  const [isSaving, setIsSaving] = useState(false);

  const isDirty =
    profile !== null &&
    (nickname !== profile.nickname ||
      email !== profile.email ||
      bio !== savedBio);

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
    if (profile === null || isSaving || !isDirty) {
      return;
    }

    setIsSaving(true);
    try {
      const updated = await apiRequest<ProfileUpdateResponse>(
        "/api/v1/users/me",
        { method: "PATCH", body: { nickname, email, bio } },
      );
      setNickname(updated.nickname);
      setEmail(updated.email);
      setSavedBio(bio);
      toast.success(t("saved"));
      router.refresh();
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
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
    bio,
    isDirty,
    isSaving,
    changeNickname: setNickname,
    changeEmail: setEmail,
    changeBio: setBio,
    save,
  };
}
