"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Ellipsis, LogOut, MonitorSmartphone, ServerOff } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import {
  ConsoleListTable,
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
import { ApiError, looksLikeServiceFailure } from "@/lib/api";
import { formatTimestamp } from "@/lib/datetime";
import { describeUserAgent } from "@/lib/user-agent";
import {
  fetchUserSessions,
  revokeUserSession,
  type UserSession,
} from "@/lib/user-sessions";

const missingValue = "—";

const sessionSortFields = ["lastSeenAt", "createdAt", "expiresAt"] as const;

type SessionSortField = (typeof sessionSortFields)[number];

function isSessionSortField(value: string): value is SessionSortField {
  return (sessionSortFields as readonly string[]).includes(value);
}

type SessionRowActionsProperties = {
  session: UserSession;
  isBusy: boolean;
  onRevoke: () => void;
};

function SessionRowActions({
  session,
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
        title={t("current")}
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
            aria-label={t("actions")}
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
  const router = useRouter();
  const [sessions, setSessions] = useState(initialSessions);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [searchText, setSearchText] = useState("");
  const [sortField, setSortField] = useState<SessionSortField>("lastSeenAt");
  const [sortDirection, setSortDirection] =
    useState<ConsoleListSortDirection>("desc");
  const [revokingSessionId, setRevokingSessionId] = useState<string | null>(
    null,
  );
  const [pendingRevoke, setPendingRevoke] = useState<UserSession | null>(null);

  const isBusy = revokingSessionId !== null;
  const isUnavailable = sessions === null;

  function describeDevice(session: UserSession): string {
    const { browser, os } = describeUserAgent(session.userAgent);
    const parts = [browser, os].filter((part): part is string => part !== null);
    return parts.length === 0 ? t("unknownDevice") : parts.join(" / ");
  }

  function reportFailure(error: unknown, fallbackMessage: string): void {
    toast.error(
      looksLikeServiceFailure(error)
        ? tCommon("serviceUnavailable")
        : fallbackMessage,
    );
  }

  async function refreshSessions(): Promise<void> {
    if (isRefreshing || isBusy) {
      return;
    }

    setIsRefreshing(true);
    try {
      setSessions(await fetchUserSessions());
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        router.replace("/signin");
        return;
      }
      reportFailure(error, t("loadFailedTitle"));
    } finally {
      setIsRefreshing(false);
    }
  }

  async function revokeSession(session: UserSession): Promise<void> {
    setRevokingSessionId(session.id);
    try {
      await revokeUserSession(session.id);
      setSessions(
        (previous) =>
          previous?.filter((row) => row.id !== session.id) ?? previous,
      );
      toast.success(t("revoked"));
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        router.replace("/signin");
        return;
      }
      reportFailure(error, t("revokeFailed"));
    } finally {
      setRevokingSessionId(null);
    }
  }

  async function confirmRevoke(): Promise<void> {
    const target = pendingRevoke;
    setPendingRevoke(null);
    if (target !== null) {
      await revokeSession(target);
    }
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

      return [describeDevice(session), session.userAgent, session.ip].some(
        (searchablePart) =>
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
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-muted/50 text-muted-foreground">
            <MonitorSmartphone className="size-4" aria-hidden="true" />
          </span>
          <span className="min-w-0">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate text-sm font-medium">
                {describeDevice(session)}
              </span>
              {session.current ? (
                <Badge variant="secondary">{t("current")}</Badge>
              ) : null}
            </span>
            <span className="block truncate font-mono text-xs text-muted-foreground">
              {session.ip === "" ? t("unknownIp") : session.ip}
            </span>
          </span>
        </span>
      ),
    },
    {
      key: "lastSeenAt",
      header: t("lastSeen"),
      sortField: "lastSeenAt",
      headerClassName: "px-4",
      cellClassName: "px-4 text-xs text-muted-foreground",
      render: (session) => formatTimestamp(session.lastSeenAt) || missingValue,
    },
    {
      key: "createdAt",
      header: t("signedIn"),
      sortField: "createdAt",
      headerClassName: "px-4",
      cellClassName: "px-4 text-xs text-muted-foreground",
      render: (session) => formatTimestamp(session.createdAt) || missingValue,
    },
    {
      key: "expiresAt",
      header: t("expiresAt"),
      sortField: "expiresAt",
      headerClassName: "px-4",
      cellClassName: "px-4 text-xs text-muted-foreground",
      render: (session) => formatTimestamp(session.expiresAt) || missingValue,
    },
  ];

  const emptyState = isUnavailable ? (
    <Empty className="py-12">
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
        {t("retry")}
      </Button>
    </Empty>
  ) : (
    <Empty className="py-12">
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
          isBusy={isBusy}
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
              isBusy={isBusy}
              onRevoke={() => setPendingRevoke(session)}
            />
          )}
          actionsHeader={t("actions")}
          emptyState={emptyState}
        />
      </Card>

      <AlertDialog
        open={pendingRevoke !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) {
            setPendingRevoke(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <LogOut aria-hidden="true" />
            </AlertDialogMedia>
            <AlertDialogTitle>{t("revokeConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("revokeConfirmDescription", {
                device:
                  pendingRevoke === null ? "" : describeDevice(pendingRevoke),
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isBusy}>
              {tCommon("cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={isBusy}
              onClick={() => void confirmRevoke()}
            >
              {t("revoke")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
