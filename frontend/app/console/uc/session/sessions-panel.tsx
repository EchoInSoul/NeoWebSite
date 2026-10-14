"use client";

import { useState } from "react";
import { Ellipsis, LogOut, MonitorSmartphone, ServerOff } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  ConsoleListTable,
  ConsoleListTitleBlock,
  type ConsoleListColumn,
  type ConsoleListSortDirection,
} from "@/components/console/console-list-table";
import {
  ConsoleListSearchInput,
  ConsoleListToolbar,
} from "@/components/console/console-list-toolbar";
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
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { useSessions } from "@/hooks/use-sessions";
import { formatTimestamp } from "@/lib/datetime";
import { describeUserAgent } from "@/lib/user-agent";
import type { UserSession } from "@/lib/user-sessions";

const missingValue = "—";
const deviceChipClassName =
  "flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-muted/50 text-muted-foreground";
const timeCellClassName = "px-4 text-xs text-muted-foreground";

const sessionSortFields = ["lastSeenAt", "createdAt", "expiresAt"] as const;

type SessionSortField = (typeof sessionSortFields)[number];

function isSessionSortField(value: string): value is SessionSortField {
  return (sessionSortFields as readonly string[]).includes(value);
}

function describeDeviceLabel(
  userAgent: string,
  unknownDeviceLabel: string,
): string {
  const { browser, os } = describeUserAgent(userAgent);
  const parts = [browser, os].filter((part): part is string => part !== null);
  return parts.length === 0 ? unknownDeviceLabel : parts.join(" / ");
}

type SessionRowActionsProperties = {
  session: UserSession;
  deviceLabel: string;
  isBusy: boolean;
  onRevoke: () => void;
};

function SessionRowActions({
  session,
  deviceLabel,
  isBusy,
  onRevoke,
}: SessionRowActionsProperties) {
  const t = useTranslations("sessions");

  if (session.current) {
    return (
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        disabled
        aria-label={t("current")}
      >
        <Ellipsis aria-hidden="true" />
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={isBusy}
            aria-label={t("actionsForDevice", { device: deviceLabel })}
          />
        }
      >
        <Ellipsis aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          variant="destructive"
          disabled={isBusy}
          onClick={onRevoke}
        >
          <LogOut aria-hidden="true" />
          <span>{t("revoke")}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function SessionsPanel({
  initialSessions,
}: {
  initialSessions: UserSession[] | null;
}) {
  const t = useTranslations("sessions");
  const tCommon = useTranslations("common");
  const {
    sessions,
    isRefreshing,
    isListLocked,
    isConfirmOpen,
    revokeTarget,
    setConfirmOpen,
    refreshSessions,
    requestRevoke,
    confirmRevoke,
  } = useSessions(initialSessions);
  const [searchText, setSearchText] = useState("");
  const [sortField, setSortField] = useState<SessionSortField>("lastSeenAt");
  const [sortDirection, setSortDirection] =
    useState<ConsoleListSortDirection>("desc");

  const isUnavailable = sessions === null;

  function describeDevice(session: UserSession): string {
    return describeDeviceLabel(session.userAgent, t("unknownDevice"));
  }

  function toggleSort(nextSortField: string): void {
    if (!isSessionSortField(nextSortField)) {
      return;
    }

    if (nextSortField === sortField) {
      setSortDirection((previous) => (previous === "asc" ? "desc" : "asc"));
      return;
    }

    setSortField(nextSortField);
    setSortDirection("desc");
  }

  const searchKeyword = searchText.trim().toLowerCase();
  const visibleSessions = (sessions ?? [])
    .filter((session) => {
      if (searchKeyword.length === 0) {
        return true;
      }

      return [describeDevice(session), session.ip].some((searchablePart) =>
        searchablePart.toLowerCase().includes(searchKeyword),
      );
    })
    .sort((left, right) => {
      const leftValue = left[sortField];
      const rightValue = right[sortField];

      if (leftValue === rightValue) {
        return 0;
      }

      return (
        (leftValue < rightValue ? -1 : 1) * (sortDirection === "asc" ? 1 : -1)
      );
    });

  const columns: readonly ConsoleListColumn<UserSession>[] = [
    {
      key: "device",
      header: t("device"),
      headerClassName: "px-4",
      cellClassName: "px-4",
      render: (session) => (
        <span className="flex min-w-0 items-center gap-3">
          <span className={deviceChipClassName}>
            <MonitorSmartphone className="size-4" aria-hidden="true" />
          </span>
          <ConsoleListTitleBlock
            title={describeDevice(session)}
            trailing={
              session.current ? (
                <Badge variant="secondary">{t("current")}</Badge>
              ) : undefined
            }
            secondary={session.ip === "" ? t("unknownIp") : session.ip}
          />
        </span>
      ),
    },
    {
      key: "lastSeenAt",
      header: t("lastSeen"),
      sortField: "lastSeenAt",
      headerClassName: "px-4",
      cellClassName: timeCellClassName,
      render: (session) => formatTimestamp(session.lastSeenAt) || missingValue,
    },
    {
      key: "createdAt",
      header: t("signedIn"),
      sortField: "createdAt",
      headerClassName: "px-4",
      cellClassName: timeCellClassName,
      render: (session) => formatTimestamp(session.createdAt) || missingValue,
    },
    {
      key: "expiresAt",
      header: t("expiresAt"),
      sortField: "expiresAt",
      headerClassName: "px-4",
      cellClassName: timeCellClassName,
      render: (session) => formatTimestamp(session.expiresAt) || missingValue,
    },
  ];

  const emptyState = isUnavailable ? (
    <Empty size="lg">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <ServerOff aria-hidden="true" />
        </EmptyMedia>
        <EmptyTitle>{t("loadFailedTitle")}</EmptyTitle>
        <EmptyDescription>{t("loadFailedDescription")}</EmptyDescription>
      </EmptyHeader>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={isRefreshing}
        onClick={() => void refreshSessions()}
      >
        {tCommon("retry")}
      </Button>
    </Empty>
  ) : (
    <Empty size="lg">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <MonitorSmartphone aria-hidden="true" />
        </EmptyMedia>
        <EmptyTitle>
          {searchKeyword.length === 0
            ? t("emptyTitle")
            : t("emptyFilteredTitle")}
        </EmptyTitle>
        <EmptyDescription>
          {searchKeyword.length === 0
            ? t("emptyDescription")
            : t("emptyFilteredDescription")}
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  );

  return (
    <>
      <Card size="none">
        <ConsoleListToolbar
          start={
            <ConsoleListSearchInput
              value={searchText}
              onValueChange={setSearchText}
              placeholder={t("searchPlaceholder")}
              disabled={isUnavailable}
            />
          }
          isBusy={isListLocked}
          isRefreshing={isRefreshing}
          onRefresh={() => void refreshSessions()}
          refreshLabel={tCommon("refresh")}
        />
        <ConsoleListTable
          rows={visibleSessions}
          columns={columns}
          getRowKey={(session) => session.id}
          sortField={sortField}
          sortDirection={sortDirection}
          onToggleSort={toggleSort}
          rowActions={(session) => (
            <SessionRowActions
              session={session}
              deviceLabel={describeDevice(session)}
              isBusy={isListLocked}
              onRevoke={() => requestRevoke(session)}
            />
          )}
          actionsHeader={t("actions")}
          emptyState={emptyState}
        />
      </Card>

      <AlertDialog open={isConfirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <LogOut aria-hidden="true" />
            </AlertDialogMedia>
            <AlertDialogTitle>{t("revokeConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("revokeConfirmDescription", {
                device:
                  revokeTarget === null ? "" : describeDevice(revokeTarget),
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{tCommon("cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void confirmRevoke()}>
              {t("revoke")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
