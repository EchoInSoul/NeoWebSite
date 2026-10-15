import { useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  groupErrorKey,
  parseAdminGroup,
  parseAdminGroups,
  type AdminGroup,
} from "@/lib/groups";

export function useGroups(initialGroups: AdminGroup[]) {
  const t = useTranslations("groups");
  const tCommon = useTranslations("common");
  const [groups, setGroups] = useState(initialGroups);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<AdminGroup | null>(null);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);

  const isListLocked = isSaving || isDeleting || isRefreshing;

  function removeGroup(groupId: string): void {
    setGroups((previous) => previous.filter((row) => row.id !== groupId));
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

  function describeError(
    error: unknown,
    context: "create" | "edit" | "delete",
  ): string {
    const code = error instanceof ApiError ? error.code : "";
    return t(groupErrorKey(code, context));
  }

  async function refreshGroups(): Promise<void> {
    if (isListLocked) {
      return;
    }

    setIsRefreshing(true);
    try {
      const payload = await apiRequest("/api/v1/admin/groups");
      const parsed = parseAdminGroups(payload);

      if (parsed === null) {
        toast.error(t("loadFailed"));
        return;
      }

      setGroups(parsed);
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

  async function submitGroup(input: {
    groupId: string | null;
    name: string;
    permissions: string[];
  }): Promise<boolean> {
    if (isListLocked) {
      return false;
    }

    const { groupId } = input;
    setIsSaving(true);
    try {
      const payload = await apiRequest(
        groupId === null
          ? "/api/v1/admin/groups"
          : `/api/v1/admin/groups/${encodeURIComponent(groupId)}`,
        {
          method: groupId === null ? "POST" : "PATCH",
          body: { name: input.name, permissions: input.permissions },
        },
      );
      const group = parseAdminGroup(payload);

      if (group === null) {
        toast.error(t(groupId === null ? "createFailed" : "saveFailed"));
        return false;
      }

      if (groupId === null) {
        setGroups((previous) => [...previous, group]);
        toast.success(t("created"));
      } else {
        setGroups((previous) =>
          previous.map((row) => (row.id === group.id ? group : row)),
        );
        toast.success(t("saved"));
      }

      return true;
    } catch (error) {
      if (handleUnauthorized(error)) {
        return false;
      }

      if (
        groupId !== null &&
        error instanceof ApiError &&
        error.status === 404
      ) {
        removeGroup(groupId);
        toast.error(t("groupMissing"));
        return true;
      }

      reportFailure(
        error,
        describeError(error, groupId === null ? "create" : "edit"),
      );
      return false;
    } finally {
      setIsSaving(false);
    }
  }

  function requestDelete(group: AdminGroup): void {
    setDeleteTarget(group);
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
      await apiRequest(`/api/v1/admin/groups/${encodeURIComponent(targetId)}`, {
        method: "DELETE",
      });
      removeGroup(targetId);
      toast.success(t("deleted"));
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }
      if (error instanceof ApiError && error.status === 404) {
        removeGroup(targetId);
        toast.success(t("deleted"));
        return;
      }
      reportFailure(error, describeError(error, "delete"));
    } finally {
      setIsDeleting(false);
    }
  }

  return {
    groups,
    isRefreshing,
    isSaving,
    isDeleting,
    isListLocked,
    isDeleteOpen,
    setDeleteOpen: setIsDeleteOpen,
    refreshGroups,
    submitGroup,
    requestDelete,
    confirmDelete,
  };
}
