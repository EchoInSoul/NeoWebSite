import { useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  deleteBlockReason,
  parseAdminUsers,
  userDeleteErrorKey,
  type AdminUser,
} from "@/lib/users";

export function useUsers(
  initialUsers: AdminUser[],
  blockContext: { currentUserEmail: string; viewerIsOwner: boolean },
) {
  const t = useTranslations("users");
  const tCommon = useTranslations("common");
  const [users, setUsers] = useState(initialUsers);
  const [selectedKeys, setSelectedKeys] = useState<ReadonlySet<string>>(
    new Set(),
  );
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isBatchDeleteOpen, setIsBatchDeleteOpen] = useState(false);

  const isListLocked = isDeleting || isRefreshing;

  const selectableKeys = users
    .filter((user) => deleteBlockReason(user, blockContext) === null)
    .map((user) => user.id);
  const isAllSelected =
    selectableKeys.length > 0 &&
    selectableKeys.every((key) => selectedKeys.has(key));
  const isSomeSelected =
    selectableKeys.some((key) => selectedKeys.has(key)) && !isAllSelected;

  function removeUsers(userIds: string[]): void {
    const removed = new Set(userIds);
    setUsers((previous) => previous.filter((row) => !removed.has(row.id)));
    setSelectedKeys(
      (previous) => new Set([...previous].filter((key) => !removed.has(key))),
    );
  }

  function toggleRowSelected(key: string, checked: boolean): void {
    setSelectedKeys((previous) => {
      const next = new Set(previous);
      if (checked) {
        next.add(key);
      } else {
        next.delete(key);
      }
      return next;
    });
  }

  function toggleAllSelected(checked: boolean): void {
    setSelectedKeys(checked ? new Set(selectableKeys) : new Set());
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
      const present = new Set(parsed.map((user) => user.id));
      setSelectedKeys(
        (previous) => new Set([...previous].filter((key) => present.has(key))),
      );
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
      removeUsers([targetId]);
      toast.success(t("deleted"));
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }

      if (error instanceof ApiError && error.status === 404) {
        removeUsers([targetId]);
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

  function requestDeleteSelected(): void {
    if (selectedKeys.size === 0) {
      return;
    }
    setIsBatchDeleteOpen(true);
  }

  async function confirmDeleteSelected(): Promise<void> {
    if (selectedKeys.size === 0 || isListLocked) {
      return;
    }

    const ids = Array.from(selectedKeys);
    setIsBatchDeleteOpen(false);
    setIsDeleting(true);
    try {
      await apiRequest("/api/v1/admin/users", {
        method: "DELETE",
        body: { ids },
      });
      removeUsers(ids);
      toast.success(t("deletedMany", { count: ids.length }));
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }

      if (error instanceof ApiError && error.status === 404) {
        toast.error(t("selectedMissing"));
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
    selectedKeys,
    isAllSelected,
    isSomeSelected,
    isRefreshing,
    isDeleting,
    isListLocked,
    isDeleteOpen,
    setDeleteOpen: setIsDeleteOpen,
    deleteTarget,
    isBatchDeleteOpen,
    setBatchDeleteOpen: setIsBatchDeleteOpen,
    toggleRowSelected,
    toggleAllSelected,
    refreshUsers,
    requestDelete,
    confirmDelete,
    requestDeleteSelected,
    confirmDeleteSelected,
  };
}
