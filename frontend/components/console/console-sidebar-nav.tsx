"use client";

import { useRef, useState } from "react";
import { flushSync } from "react-dom";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  CircleUserRound,
  MonitorSmartphone,
  Settings,
  SlidersHorizontal,
  UserRoundCog,
  UserRoundPlus,
  Users,
} from "lucide-react";
import { useTranslations } from "next-intl";

import {
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

import {
  ConsoleSidebarSection,
  type ConsoleSidebarBack,
  type ConsoleSidebarSectionGroup,
  type ConsoleSidebarSectionItem,
} from "./console-sidebar-section";

const PANEL_PREFIXES = {
  uc: "/console/uc",
  settings: "/console/settings",
} as const;

const SETTINGS_GENERAL_HREF = `${PANEL_PREFIXES.settings}/general`;
const SETTINGS_REGISTER_HREF = `${PANEL_PREFIXES.settings}/register`;
const SETTINGS_GROUPS_HREF = `${PANEL_PREFIXES.settings}/groups`;
const SETTINGS_USERS_HREF = `${PANEL_PREFIXES.settings}/users`;

type PanelKey = "main" | keyof typeof PANEL_PREFIXES;

type ConsoleSidebarPanelContent = {
  title: string;
  tooltip: string;
  back: ConsoleSidebarBack;
  groups: ConsoleSidebarSectionGroup[];
};

function isUnder(pathname: string, base: string) {
  return pathname === base || pathname.startsWith(`${base}/`);
}

function resolvePanel(pathname: string): PanelKey {
  if (isUnder(pathname, PANEL_PREFIXES.uc)) {
    return "uc";
  }
  if (isUnder(pathname, PANEL_PREFIXES.settings)) {
    return "settings";
  }
  return "main";
}

export function ConsoleSidebarNav({
  canViewSettings = false,
  canViewRegister = false,
  canViewGroups = false,
  canViewUsers = false,
}: {
  canViewSettings?: boolean;
  canViewRegister?: boolean;
  canViewGroups?: boolean;
  canViewUsers?: boolean;
}) {
  const t = useTranslations("sidebar");
  const ts = useTranslations("settings");
  const pathname = usePathname();

  const settingsEntryRef = useRef<HTMLAnchorElement>(null);

  const hasSettingsAccess =
    canViewSettings || canViewRegister || canViewGroups || canViewUsers;

  const routePanel = resolvePanel(pathname);
  const [displayedPanel, setDisplayedPanel] = useState<PanelKey>(routePanel);
  const [syncedRoutePanel, setSyncedRoutePanel] =
    useState<PanelKey>(routePanel);

  if (routePanel !== syncedRoutePanel) {
    setSyncedRoutePanel(routePanel);
    setDisplayedPanel(routePanel);
  }

  const panels: Record<
    Exclude<PanelKey, "main">,
    ConsoleSidebarPanelContent
  > = {
    uc: {
      title: t("profile"),
      tooltip: t("backToConsole"),
      back: { kind: "link", href: "/console" },
      groups: [
        {
          label: t("account"),
          items: [
            {
              href: "/console/uc/profile",
              label: t("personalInfo"),
              icon: <CircleUserRound />,
              className: "mb-1",
            },
            {
              href: "/console/uc/session",
              label: t("sessions"),
              icon: <MonitorSmartphone />,
            },
          ],
        },
      ],
    },
    settings: {
      title: t("settings"),
      tooltip: t("backToConsole"),
      back: {
        kind: "button",
        onClick: () => {
          flushSync(() => {
            setDisplayedPanel("main");
          });
          settingsEntryRef.current?.focus();
        },
      },
      groups: [
        {
          label: t("settings"),
          items: canViewSettings
            ? [
                {
                  href: SETTINGS_GENERAL_HREF,
                  label: ts("general"),
                  icon: <SlidersHorizontal />,
                } satisfies ConsoleSidebarSectionItem,
              ]
            : [],
        },
        {
          label: t("signInAndRegister"),
          items: canViewRegister
            ? [
                {
                  href: SETTINGS_REGISTER_HREF,
                  label: ts("register"),
                  icon: <UserRoundPlus />,
                } satisfies ConsoleSidebarSectionItem,
              ]
            : [],
        },
        {
          label: t("users"),
          items: [
            ...(canViewUsers
              ? [
                  {
                    href: SETTINGS_USERS_HREF,
                    label: ts("users"),
                    icon: <UserRoundCog />,
                    className: canViewGroups ? "mb-1" : undefined,
                  } satisfies ConsoleSidebarSectionItem,
                ]
              : []),
            ...(canViewGroups
              ? [
                  {
                    href: SETTINGS_GROUPS_HREF,
                    label: ts("groups"),
                    icon: <Users />,
                  } satisfies ConsoleSidebarSectionItem,
                ]
              : []),
          ],
        },
      ].filter((group) => group.items.length > 0),
    },
  };

  if (displayedPanel === "main") {
    return (
      <SidebarGroup>
        <SidebarMenu>
          {hasSettingsAccess ? (
            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={
                  displayedPanel === "main" && routePanel === "settings"
                }
                render={
                  <Link
                    ref={settingsEntryRef}
                    href={SETTINGS_GENERAL_HREF}
                    onClick={() => setDisplayedPanel("settings")}
                  />
                }
                tooltip={t("settings")}
              >
                <Settings />
                <span>{t("settings")}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ) : null}
        </SidebarMenu>
      </SidebarGroup>
    );
  }

  const panel = panels[displayedPanel];

  return (
    <ConsoleSidebarSection
      title={panel.title}
      tooltip={panel.tooltip}
      back={panel.back}
      groups={panel.groups}
    />
  );
}
