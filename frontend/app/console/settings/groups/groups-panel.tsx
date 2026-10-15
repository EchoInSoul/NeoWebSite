"use client";

import { useState } from "react";
import { Ellipsis, Pencil, Plus, Trash2, Users } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  ConsoleListTable,
  ConsoleListTitleBlock,
  type ConsoleListColumn,
  type ConsoleListSortDirection,
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useGroups } from "@/hooks/use-groups";
import {
  isDisplayedPermission,
  sortGroupsForDisplay,
  type AdminGroup,
  type PermissionMatrix,
} from "@/lib/groups";

import { GroupDialog } from "./group-dialog";

const SORT_FIELDS = ["memberCount", "permissions"] as const;

type GroupSortField = (typeof SORT_FIELDS)[number];

function isGroupSortField(value: string): value is GroupSortField {
  return (SORT_FIELDS as readonly string[]).includes(value);
}

function permissionCount(group: AdminGroup): number {
  return group.permissions.filter(isDisplayedPermission).length;
}

type RowActionsProperties = {
  group: AdminGroup;
  isBusy: boolean;
  onEdit: (group: AdminGroup) => void;
  onDelete: (group: AdminGroup) => void;
};

function RowActions({ group, isBusy, onEdit, onDelete }: RowActionsProperties) {
  const t = useTranslations("groups");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={isBusy}
          />
        }
        aria-label={t("actionsMenu", { name: group.name })}
        title={t("actionsMenu", { name: group.name })}
      >
        <Ellipsis aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem disabled={isBusy} onClick={() => onEdit(group)}>
          <Pencil aria-hidden="true" />
          {t("edit")}
        </DropdownMenuItem>
        {group.isPreset ? null : (
          <DropdownMenuItem
            variant="destructive"
            disabled={isBusy || group.memberCount > 0}
            title={group.memberCount > 0 ? t("deleteBlocked") : undefined}
            onClick={() => onDelete(group)}
          >
            <Trash2 aria-hidden="true" />
            {t("delete")}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function GroupsPanel({
  initialGroups,
  matrix,
  canEdit,
}: {
  initialGroups: AdminGroup[];
  matrix: PermissionMatrix;
  canEdit: boolean;
}) {
  const t = useTranslations("groups");
  const tCommon = useTranslations("common");
  const {
    groups,
    isRefreshing,
    isListLocked,
    isDeleteOpen,
    setDeleteOpen,
    refreshGroups,
    submitGroup,
    requestDelete,
    confirmDelete,
  } = useGroups(initialGroups);
  const [sortField, setSortField] = useState<GroupSortField | null>(null);
  const [sortDirection, setSortDirection] =
    useState<ConsoleListSortDirection>("desc");
  const [dialog, setDialog] = useState<{
    group: AdminGroup | null;
    sequence: number;
  } | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState(false);

  let visibleGroups = sortGroupsForDisplay(groups);
  if (sortField !== null) {
    visibleGroups = [...visibleGroups].sort((left, right) => {
      const leftValue =
        sortField === "memberCount" ? left.memberCount : permissionCount(left);
      const rightValue =
        sortField === "memberCount"
          ? right.memberCount
          : permissionCount(right);
      if (leftValue === rightValue) {
        return 0;
      }
      return (
        (leftValue < rightValue ? -1 : 1) * (sortDirection === "asc" ? 1 : -1)
      );
    });
  }

  function toggleSort(nextSortField: string) {
    if (!isGroupSortField(nextSortField)) {
      return;
    }

    if (nextSortField === sortField) {
      setSortDirection((previous) => (previous === "asc" ? "desc" : "asc"));
      return;
    }

    setSortField(nextSortField);
    setSortDirection("desc");
  }

  function openDialog(group: AdminGroup | null) {
    setDialog((previous) => ({
      group,
      sequence: (previous?.sequence ?? 0) + 1,
    }));
    setIsDialogOpen(true);
  }

  const columns: readonly ConsoleListColumn<AdminGroup>[] = [
    {
      key: "name",
      header: t("columnName"),
      headerClassName: "px-4",
      cellClassName: "px-4",
      render: (group) => (
        <span className="flex min-w-0 items-center gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <Users className="size-[18px]" aria-hidden="true" />
          </span>
          <ConsoleListTitleBlock
            title={group.name}
            trailing={
              group.isPreset ? (
                <Badge variant="secondary">{t("preset")}</Badge>
              ) : undefined
            }
          />
        </span>
      ),
    },
    {
      key: "members",
      header: t("columnMembers"),
      sortField: "memberCount",
      headerClassName: "px-4",
      cellClassName: "px-4 text-xs text-muted-foreground",
      render: (group) => group.memberCount,
    },
    {
      key: "permissions",
      header: t("columnPermissions"),
      sortField: "permissions",
      headerClassName: "px-4",
      cellClassName: "px-4 text-xs text-muted-foreground",
      render: (group) => permissionCount(group),
    },
  ];

  return (
    <>
      <Card size="none">
        <ConsoleListToolbar
          start={
            canEdit ? (
              <Button
                type="button"
                variant="outline"
                size="default"
                disabled={isListLocked}
                onClick={() => openDialog(null)}
              >
                <Plus aria-hidden="true" />
                {t("newGroup")}
              </Button>
            ) : undefined
          }
          isBusy={isListLocked}
          isRefreshing={isRefreshing}
          onRefresh={() => void refreshGroups()}
          refreshLabel={tCommon("refresh")}
        />
        <ConsoleListTable
          rows={visibleGroups}
          columns={columns}
          getRowKey={(group) => group.id}
          sortField={sortField ?? undefined}
          sortDirection={sortDirection}
          onToggleSort={toggleSort}
          rowActions={
            canEdit
              ? (group) => (
                  <RowActions
                    group={group}
                    isBusy={isListLocked}
                    onEdit={openDialog}
                    onDelete={requestDelete}
                  />
                )
              : undefined
          }
          actionsHeader={canEdit ? t("actions") : undefined}
          emptyState={
            <p className="px-4 py-8 text-center text-sm text-muted-foreground">
              {t("empty")}
            </p>
          }
        />
      </Card>

      {dialog === null ? null : (
        <GroupDialog
          key={dialog.sequence}
          open={isDialogOpen}
          onOpenChange={(open) => {
            if (!open) {
              setIsDialogOpen(false);
            }
          }}
          group={dialog.group}
          matrix={matrix}
          onSubmit={(input) =>
            submitGroup({ groupId: dialog.group?.id ?? null, ...input })
          }
        />
      )}

      <AlertDialog open={isDeleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <Trash2 aria-hidden="true" />
            </AlertDialogMedia>
            <AlertDialogTitle>{t("deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("deleteConfirmDescription")}
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
