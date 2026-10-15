"use client";

import { useRef, useState } from "react";
import { Ellipsis, Pencil, Plus, Trash2, Users } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

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
import {
  buildPermissionMatrix,
  sortGroupsForDisplay,
  type AdminGroup,
  type PermissionCatalogItem,
} from "@/lib/groups";

import { GroupDialog } from "./group-dialog";

const CATALOG: PermissionCatalogItem[] = [
  { key: "system.users.view", name: "查看用户" },
  { key: "system.users.edit", name: "管理用户" },
  { key: "system.groups.view", name: "查看用户组" },
  { key: "system.groups.edit", name: "管理用户组" },
  { key: "system.settings.view", name: "查看设置" },
  { key: "system.settings.edit", name: "管理设置" },
];

const PERMISSION_MATRIX = buildPermissionMatrix(CATALOG);

const SORT_FIELDS = ["memberCount", "permissions"] as const;

type GroupSortField = (typeof SORT_FIELDS)[number];

function isGroupSortField(value: string): value is GroupSortField {
  return (SORT_FIELDS as readonly string[]).includes(value);
}

type RowActionsProperties = {
  group: AdminGroup;
  onEdit: (group: AdminGroup) => void;
  onDelete: (group: AdminGroup) => void;
};

function RowActions({ group, onEdit, onDelete }: RowActionsProperties) {
  const t = useTranslations("groups");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button type="button" variant="ghost" size="icon-sm" />}
        aria-label={t("actionsMenu", { name: group.name })}
        title={t("actionsMenu", { name: group.name })}
      >
        <Ellipsis aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={() => onEdit(group)}>
          <Pencil aria-hidden="true" />
          {t("edit")}
        </DropdownMenuItem>
        {group.isPreset ? null : (
          <DropdownMenuItem
            variant="destructive"
            disabled={group.memberCount > 0}
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

export function GroupsPanel() {
  const t = useTranslations("groups");
  const tCommon = useTranslations("common");
  const [groups, setGroups] = useState<AdminGroup[]>([]);
  const [sortField, setSortField] = useState<GroupSortField | null>(null);
  const [sortDirection, setSortDirection] =
    useState<ConsoleListSortDirection>("desc");
  const [dialog, setDialog] = useState<{
    group: AdminGroup | null;
    sequence: number;
  } | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<AdminGroup | null>(null);
  const nextGroupId = useRef(1);

  let visibleGroups = sortGroupsForDisplay(groups);
  if (sortField !== null) {
    visibleGroups = [...visibleGroups].sort((left, right) => {
      const leftValue =
        sortField === "memberCount"
          ? left.memberCount
          : left.permissions.length;
      const rightValue =
        sortField === "memberCount"
          ? right.memberCount
          : right.permissions.length;
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

  async function submitGroup(input: {
    name: string;
    permissions: string[];
  }): Promise<boolean> {
    const name = input.name.trim();
    const current = dialog?.group ?? null;
    const duplicated = groups.some(
      (group) => group !== current && group.name === name,
    );

    if (duplicated) {
      toast.error(t("nameTaken"));
      return false;
    }

    if (current === null) {
      setGroups((previous) => [
        ...previous,
        {
          id: `group-${nextGroupId.current++}`,
          systemKey: null,
          name,
          isPreset: false,
          memberCount: 0,
          permissions: input.permissions,
        },
      ]);
      toast.success(t("created"));
    } else {
      setGroups((previous) =>
        previous.map((group) =>
          group.id === current.id
            ? { ...group, name, permissions: input.permissions }
            : group,
        ),
      );
      toast.success(t("saved"));
    }

    return true;
  }

  function requestDelete(group: AdminGroup) {
    setDeleteTarget(group);
  }

  function confirmDelete() {
    if (deleteTarget === null) {
      return;
    }

    setGroups((previous) =>
      previous.filter((group) => group.id !== deleteTarget.id),
    );
    setDeleteTarget(null);
    toast.success(t("deleted"));
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
      render: (group) => group.permissions.length,
    },
  ];

  return (
    <>
      <Card size="none">
        <ConsoleListToolbar
          end={
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => openDialog(null)}
            >
              <Plus aria-hidden="true" />
              {t("newGroup")}
            </Button>
          }
          onRefresh={() => {}}
          refreshLabel={tCommon("refresh")}
        />
        <ConsoleListTable
          rows={visibleGroups}
          columns={columns}
          getRowKey={(group) => group.id}
          sortField={sortField ?? undefined}
          sortDirection={sortDirection}
          onToggleSort={toggleSort}
          rowActions={(group) => (
            <RowActions
              group={group}
              onEdit={openDialog}
              onDelete={requestDelete}
            />
          )}
          actionsHeader={t("actions")}
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
          matrix={PERMISSION_MATRIX}
          onSubmit={submitGroup}
        />
      )}

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
          }
        }}
      >
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
            <AlertDialogAction variant="destructive" onClick={confirmDelete}>
              {t("delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
