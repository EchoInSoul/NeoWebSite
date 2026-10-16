"use client";

import { Trash2, UserRound } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  ConsoleListTable,
  ConsoleListTitleBlock,
  type ConsoleListColumn,
} from "@/components/console/console-list-table";
import { ConsoleListToolbar } from "@/components/console/console-list-toolbar";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useUsers } from "@/hooks/use-users";
import { deleteBlockReason, type AdminUser } from "@/lib/users";

type RowActionsProperties = {
  user: AdminUser;
  isBusy: boolean;
  currentUserEmail: string;
  viewerIsOwner: boolean;
  onDelete: (user: AdminUser) => void;
};

function RowActions({
  user,
  isBusy,
  currentUserEmail,
  viewerIsOwner,
  onDelete,
}: RowActionsProperties) {
  const t = useTranslations("users");
  const blockReason = deleteBlockReason(user, {
    currentUserEmail,
    viewerIsOwner,
  });
  const label = t("deleteAction", { name: user.nickname });

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      disabled={isBusy || blockReason !== null}
      aria-label={blockReason === null ? label : t(blockReason)}
      title={blockReason === null ? label : t(blockReason)}
      onClick={() => onDelete(user)}
    >
      <Trash2 aria-hidden="true" />
    </Button>
  );
}

export function UsersPanel({
  initialUsers,
  currentUserEmail,
  viewerIsOwner,
  canDelete,
}: {
  initialUsers: AdminUser[];
  currentUserEmail: string;
  viewerIsOwner: boolean;
  canDelete: boolean;
}) {
  const t = useTranslations("users");
  const tCommon = useTranslations("common");
  const {
    users,
    isRefreshing,
    isListLocked,
    isDeleteOpen,
    deleteTarget,
    setDeleteOpen,
    refreshUsers,
    requestDelete,
    confirmDelete,
  } = useUsers(initialUsers);

  const columns: readonly ConsoleListColumn<AdminUser>[] = [
    {
      key: "user",
      header: t("columnUser"),
      headerClassName: "px-4",
      cellClassName: "px-4",
      render: (user) => (
        <span className="flex min-w-0 items-center gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <UserRound className="size-[18px]" aria-hidden="true" />
          </span>
          <ConsoleListTitleBlock title={user.nickname} secondary={user.email} />
        </span>
      ),
    },
    {
      key: "group",
      header: t("columnGroup"),
      headerClassName: "px-4",
      cellClassName: "px-4 text-xs text-muted-foreground",
      render: (user) => user.group.name,
    },
  ];

  return (
    <>
      <Card size="none">
        <ConsoleListToolbar
          isBusy={isListLocked}
          isRefreshing={isRefreshing}
          onRefresh={() => void refreshUsers()}
          refreshLabel={tCommon("refresh")}
        />
        <ConsoleListTable
          rows={users}
          columns={columns}
          getRowKey={(user) => user.id}
          rowActions={
            canDelete
              ? (user) => (
                  <RowActions
                    user={user}
                    isBusy={isListLocked}
                    currentUserEmail={currentUserEmail}
                    viewerIsOwner={viewerIsOwner}
                    onDelete={requestDelete}
                  />
                )
              : undefined
          }
          actionsHeader={canDelete ? t("actions") : undefined}
          emptyState={
            <p className="px-4 py-8 text-center text-sm text-muted-foreground">
              {t("empty")}
            </p>
          }
        />
      </Card>

      <AlertDialog open={isDeleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <Trash2 aria-hidden="true" />
            </AlertDialogMedia>
            <AlertDialogTitle>{t("deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("deleteConfirmDescription", {
                name: deleteTarget === null ? "" : deleteTarget.nickname,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{tCommon("cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => void confirmDelete()}
            >
              {t("delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
