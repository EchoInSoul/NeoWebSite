import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, looksLikeServiceFailure } from "@/lib/api";
import {
  fetchUserSessions,
  revokeUserSession,
  type UserSession,
} from "@/lib/user-sessions";

export function useSessions(initialSessions: UserSession[] | null) {
  const t = useTranslations("sessions");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [sessions, setSessions] = useState(initialSessions);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [revokingSessionId, setRevokingSessionId] = useState<string | null>(
    null,
  );
  const [revokeTarget, setRevokeTarget] = useState<UserSession | null>(null);
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);

  const isRevoking = revokingSessionId !== null;
  const isListLocked = isRevoking || isRefreshing;

  function removeSession(sessionId: string): void {
    setSessions(
      (previous) => previous?.filter((row) => row.id !== sessionId) ?? previous,
    );
  }

  function reportFailure(error: unknown, fallbackMessage: string): void {
    toast.error(
      looksLikeServiceFailure(error)
        ? tCommon("serviceUnavailable")
        : fallbackMessage,
    );
  }

  function handleUnauthorized(error: unknown): boolean {
    if (error instanceof ApiError && error.status === 401) {
      router.replace("/signin");
      return true;
    }
    return false;
  }

  async function refreshSessions(): Promise<void> {
    if (isListLocked) {
      return;
    }

    setIsRefreshing(true);
    try {
      setSessions(await fetchUserSessions());
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }
      reportFailure(error, t("loadFailedTitle"));
    } finally {
      setIsRefreshing(false);
    }
  }

  async function revokeSession(session: UserSession): Promise<void> {
    if (isListLocked) {
      return;
    }

    setRevokingSessionId(session.id);
    try {
      await revokeUserSession(session.id);
      removeSession(session.id);
      toast.success(t("revoked"));
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }
      if (error instanceof ApiError && error.status === 404) {
        removeSession(session.id);
        toast.success(t("revoked"));
        return;
      }
      reportFailure(error, t("revokeFailed"));
    } finally {
      setRevokingSessionId(null);
    }
  }

  function requestRevoke(session: UserSession): void {
    setRevokeTarget(session);
    setIsConfirmOpen(true);
  }

  async function confirmRevoke(): Promise<void> {
    if (revokeTarget === null) {
      return;
    }

    setIsConfirmOpen(false);
    await revokeSession(revokeTarget);
  }

  return {
    sessions,
    isRefreshing,
    isRevoking,
    isListLocked,
    isConfirmOpen,
    revokeTarget,
    setConfirmOpen: setIsConfirmOpen,
    refreshSessions,
    requestRevoke,
    confirmRevoke,
  };
}
