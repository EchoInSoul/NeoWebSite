import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest } from "@/lib/api";
import { markSignoutSuccess } from "@/lib/signout-notice";

export function useSignout() {
  const t = useTranslations("userMenu");
  const router = useRouter();
  const [isSigningOut, setIsSigningOut] = useState(false);

  async function signOut() {
    setIsSigningOut(true);
    try {
      await apiRequest("/api/v1/sessions/current", { method: "DELETE" });
      markSignoutSuccess();
      router.replace("/signin");
      return;
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        markSignoutSuccess();
        window.location.replace("/signin");
        return;
      }
      toast.error(t("signOutFailed"));
    } finally {
      setIsSigningOut(false);
    }
  }

  return { isSigningOut, signOut };
}
