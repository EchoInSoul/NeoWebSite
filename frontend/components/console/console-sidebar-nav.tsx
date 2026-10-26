"use client";

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type TransitionEvent,
} from "react";
import { flushSync } from "react-dom";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  CircleUserRound,
  Code,
  LogIn,
  Mail,
  MonitorSmartphone,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  UserRoundCog,
  UserRoundPlus,
  Users,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { cn } from "cn";

import {
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  SIDEBAR_PANELS,
  sidebarPanelOrder,
  subscribeSidebarPanel,
  type SidebarPanel,
  type SidebarSubPanel,
} from "@/lib/sidebar-panel";
import {
  SIDEBAR_SLIDE_DURATION_MS,
  SIDEBAR_SLIDE_TRANSITION,
  sidebarSlideOffsetClass,
  type SidebarSlideOffset,
} from "@/lib/sidebar-transition";

import {
  ConsoleSidebarSection,
  type ConsoleSidebarSectionItem,
} from "./console-sidebar-section";

const PANEL_PREFIXES = {
  uc: "/console/uc",
  settings: "/console/settings",
} as const satisfies Record<SidebarSubPanel, string>;

const SETTINGS_INFO_HREF = `${PANEL_PREFIXES.settings}/info`;
const SETTINGS_DEVELOPER_HREF = `${PANEL_PREFIXES.settings}/developer`;
const SETTINGS_REGISTER_HREF = `${PANEL_PREFIXES.settings}/register`;
const SETTINGS_LOGIN_HREF = `${PANEL_PREFIXES.settings}/login`;
const SETTINGS_CAPTCHA_HREF = `${PANEL_PREFIXES.settings}/captcha`;
const SETTINGS_GROUPS_HREF = `${PANEL_PREFIXES.settings}/groups`;
const SETTINGS_USERS_HREF = `${PANEL_PREFIXES.settings}/users`;
const SETTINGS_MAIL_HREF = `${PANEL_PREFIXES.settings}/mail`;

const SLIDE_FALLBACK_MS = SIDEBAR_SLIDE_DURATION_MS + 80;

type PanelKey = SidebarPanel;

type SidebarSlideState = {
  offset: SidebarSlideOffset;
  isVisible: boolean;
  isSettled: boolean;
};

function isUnder(pathname: string, base: string) {
  return pathname === base || pathname.startsWith(`${base}/`);
}

function resolvePanel(pathname: string): PanelKey {
  const matched = (Object.keys(PANEL_PREFIXES) as SidebarSubPanel[]).find(
    (panel) => isUnder(pathname, PANEL_PREFIXES[panel]),
  );

  return matched ?? "main";
}

function resolveSlideState(
  panelKey: PanelKey,
  settledPanel: PanelKey,
  outgoingPanel: PanelKey | null,
): SidebarSlideState {
  if (panelKey === settledPanel) {
    return { offset: "none", isVisible: true, isSettled: true };
  }
  return {
    offset:
      sidebarPanelOrder(panelKey) < sidebarPanelOrder(settledPanel)
        ? "left"
        : "right",
    isVisible: panelKey === outgoingPanel,
    isSettled: false,
  };
}

function SidebarSlidePanel({
  slideState,
  isCollapsed,
  onTransitionEnd,
  children,
}: {
  slideState: SidebarSlideState;
  isCollapsed: boolean;
  onTransitionEnd: (event: TransitionEvent<HTMLDivElement>) => void;
  children: ReactNode;
}) {
  return (
    <div
      aria-hidden={!slideState.isSettled}
      inert={!slideState.isSettled}
      onTransitionEnd={onTransitionEnd}
      className={cn(
        "absolute inset-0 no-scrollbar overflow-y-auto bg-white dark:bg-neutral-950",
        slideState.isVisible
          ? "visible"
          : "invisible [content-visibility:hidden]",
        slideState.isSettled ? "z-10" : "pointer-events-none z-0",
        slideState.isVisible ? SIDEBAR_SLIDE_TRANSITION : "transition-none",
        sidebarSlideOffsetClass(slideState.offset, isCollapsed),
      )}
    >
      {children}
    </div>
  );
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
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";

  const settingsFallbackHref = canViewSettings
    ? SETTINGS_INFO_HREF
    : canViewRegister
      ? SETTINGS_REGISTER_HREF
      : canViewUsers
        ? SETTINGS_USERS_HREF
        : SETTINGS_GROUPS_HREF;

  const settingsEntryRef = useRef<HTMLAnchorElement>(null);
  const [lastSettingsHref, setLastSettingsHref] =
    useState(settingsFallbackHref);

  const hasSettingsAccess =
    canViewSettings || canViewRegister || canViewGroups || canViewUsers;

  const routePanel = resolvePanel(pathname);
  const [displayedPanel, setDisplayedPanel] = useState<PanelKey>(routePanel);
  const [syncedRoutePanel, setSyncedRoutePanel] =
    useState<PanelKey>(routePanel);
  const [settledPanel, setSettledPanel] = useState<PanelKey>(routePanel);
  const [outgoingPanel, setOutgoingPanel] = useState<PanelKey | null>(null);

  if (routePanel !== syncedRoutePanel) {
    setSyncedRoutePanel(routePanel);
    setDisplayedPanel(routePanel);
  }

  if (displayedPanel !== settledPanel) {
    setOutgoingPanel(settledPanel);
    setSettledPanel(displayedPanel);
  }

  if (
    routePanel === "settings" &&
    pathname !== PANEL_PREFIXES.settings &&
    pathname !== lastSettingsHref
  ) {
    setLastSettingsHref(pathname);
  }

  useEffect(() => subscribeSidebarPanel(setDisplayedPanel), []);

  useEffect(() => {
    if (outgoingPanel === null) {
      return;
    }
    const timeoutId = window.setTimeout(() => {
      setOutgoingPanel(null);
    }, SLIDE_FALLBACK_MS);
    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [outgoingPanel]);

  function handlePanelTransitionEnd(panelKey: PanelKey) {
    return function handleTransitionEnd(
      event: TransitionEvent<HTMLDivElement>,
    ) {
      if (event.target !== event.currentTarget) {
        return;
      }
      if (event.propertyName !== "translate") {
        return;
      }
      if (panelKey !== outgoingPanel) {
        return;
      }
      setOutgoingPanel(null);
    };
  }

  const subPanels = useMemo<Record<Exclude<PanelKey, "main">, ReactNode>>(
    () => ({
      uc: (
        <ConsoleSidebarSection
          title={t("profile")}
          tooltip={t("backToConsole")}
          back={{
            kind: "link",
            href: "/console",
            onNavigate: () => {
              flushSync(() => {
                setDisplayedPanel("main");
              });
            },
          }}
          groups={[
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
          ]}
        />
      ),
      settings: (
        <ConsoleSidebarSection
          title={t("settings")}
          tooltip={t("backToConsole")}
          back={{
            kind: "button",
            onClick: () => {
              flushSync(() => {
                setDisplayedPanel("main");
              });
              settingsEntryRef.current?.focus();
            },
          }}
          groups={[
            {
              label: t("settings"),
              items: canViewSettings
                ? [
                    {
                      href: SETTINGS_INFO_HREF,
                      label: ts("general"),
                      icon: <SlidersHorizontal />,
                      className: "mb-1",
                    } satisfies ConsoleSidebarSectionItem,
                    {
                      href: SETTINGS_DEVELOPER_HREF,
                      label: ts("developer"),
                      icon: <Code />,
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
                      className: "mb-1",
                    } satisfies ConsoleSidebarSectionItem,
                    {
                      href: SETTINGS_LOGIN_HREF,
                      label: ts("loginControl"),
                      icon: <LogIn />,
                      className: "mb-1",
                    } satisfies ConsoleSidebarSectionItem,
                    {
                      href: SETTINGS_CAPTCHA_HREF,
                      label: ts("captchaGroup"),
                      icon: <ShieldCheck />,
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
            {
              label: t("mail"),
              items: canViewSettings
                ? [
                    {
                      href: SETTINGS_MAIL_HREF,
                      label: ts("mail"),
                      icon: <Mail />,
                    } satisfies ConsoleSidebarSectionItem,
                  ]
                : [],
            },
          ].filter((group) => group.items.length > 0)}
        />
      ),
    }),
    [canViewGroups, canViewRegister, canViewSettings, canViewUsers, t, ts],
  );

  const settingsEntryHref =
    routePanel === "settings" && pathname !== PANEL_PREFIXES.settings
      ? pathname
      : lastSettingsHref;

  const settingsEntryActive =
    displayedPanel === "main" && routePanel === "settings";

  const mainPanel = useMemo(
    () => (
      <SidebarGroup>
        <SidebarMenu>
          {hasSettingsAccess ? (
            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={settingsEntryActive}
                render={
                  <Link
                    ref={settingsEntryRef}
                    href={settingsEntryHref}
                    draggable={false}
                    onNavigate={(event) => {
                      if (settingsEntryHref === pathname) {
                        event.preventDefault();
                      }
                      setDisplayedPanel("settings");
                    }}
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
    ),
    [hasSettingsAccess, pathname, settingsEntryActive, settingsEntryHref, t],
  );

  const panelContent: Record<PanelKey, ReactNode> = {
    main: mainPanel,
    settings: subPanels.settings,
    uc: subPanels.uc,
  };

  return (
    <div className="relative min-h-0 flex-1 overflow-hidden">
      {SIDEBAR_PANELS.map((panel) => (
        <SidebarSlidePanel
          key={panel}
          slideState={resolveSlideState(panel, settledPanel, outgoingPanel)}
          isCollapsed={isCollapsed}
          onTransitionEnd={handlePanelTransitionEnd(panel)}
        >
          {panelContent[panel]}
        </SidebarSlidePanel>
      ))}
    </div>
  );
}
