import { useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  parseAdminUsers,
  userDeleteErrorKey,
  type AdminUser,
} from "@/lib/users";

export function useUsers(initialUsers: AdminUser[]) {
  const t = useTranslations("users");
  const tCommon = useTranslations("common");
  const [users, setUsers] = useState(initialUsers);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);

  const isListLocked = isDeleting || isRefreshing;

  function removeUser(userId: string): void {
    setUsers((previous) => previous.filter((row) => row.id !== userId));
  }

  function reportFailure(error: unknown, fallbackMessage: string): void {
    toast.error(
      looksLikeServiceFailure(error)
        ? tCommon("serviceUnavailable")
        : fallbackMessage,
    );
  }

  function handleUnauthorized(error: unknown): boolean {
    return error instanceof ApiError && error.status === 401;
  }

  async function refreshUsers(): Promise<void> {
    if (isListLocked) {
      return;
    }

    setIsRefreshing(true);
    try {
      const payload = await apiRequest("/api/v1/admin/users");
      const parsed = parseAdminUsers(payload);

      if (parsed === null) {
        toast.error(t("loadFailed"));
        return;
      }

      setUsers(parsed);
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        toast.error(t("forbidden"));
        return;
      }
      reportFailure(error, t("loadFailed"));
    } finally {
      setIsRefreshing(false);
    }
  }

  function requestDelete(user: AdminUser): void {
    setDeleteTarget(user);
    setIsDeleteOpen(true);
  }

  async function confirmDelete(): Promise<void> {
    if (deleteTarget === null || isListLocked) {
      return;
    }

    setIsDeleteOpen(false);
    const targetId = deleteTarget.id;
    setIsDeleting(true);
    try {
      await apiRequest(`/api/v1/admin/users/${encodeURIComponent(targetId)}`, {
        method: "DELETE",
      });
      removeUser(targetId);
      toast.success(t("deleted"));
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }

      if (error instanceof ApiError && error.status === 404) {
        removeUser(targetId);
        toast.error(t("userMissing"));
        return;
      }

      reportFailure(
        error,
        t(userDeleteErrorKey(error instanceof ApiError ? error.code : "")),
      );
    } finally {
      setIsDeleting(false);
    }
  }

  return {
    users,
    isRefreshing,
    isDeleting,
    isListLocked,
    isDeleteOpen,
    setDeleteOpen: setIsDeleteOpen,
    deleteTarget,
    refreshUsers,
    requestDelete,
    confirmDelete,
  };
}
