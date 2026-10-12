import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { apiRequest } from "@/lib/api";

export function useSignout() {
  const t = useTranslations("userMenu");
  const router = useRouter();
  const [isSigningOut, setIsSigningOut] = useState(false);

  async function signOut() {
    setIsSigningOut(true);
    try {
      await apiRequest("/api/v1/auth/logout", { method: "POST" });
      router.replace("/signin");
      return;
    } catch {
      toast.error(t("signOutFailed"));
    } finally {
      setIsSigningOut(false);
    }
  }

  return { isSigningOut, signOut };
}
