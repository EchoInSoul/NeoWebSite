"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft, CircleUserRound, MonitorSmartphone } from "lucide-react";
import { useTranslations } from "next-intl";
import { cn } from "cn";

import { useSidebar } from "@/components/ui/sidebar";
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import {
  SIDEBAR_LABEL_TRANSITION,
  sidebarLabelFade,
} from "@/lib/sidebar-transition";

export function ConsoleSidebarNav() {
  const t = useTranslations("sidebar");
  const pathname = usePathname();
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";
  const isUserCenter =
    pathname === "/console/uc" || pathname.startsWith("/console/uc/");

  if (!isUserCenter) {
    return null;
  }

  const isPersonalInfoActive =
    pathname === "/console/uc/profile" ||
    pathname.startsWith("/console/uc/profile/");
  const isSessionsActive =
    pathname === "/console/uc/session" ||
    pathname.startsWith("/console/uc/session/");

  return (
    <SidebarGroup>
      <SidebarMenu>
        <SidebarMenuItem className="mb-2">
          <SidebarMenuButton
            render={<Link href="/console" />}
            tooltip={t("backToConsole")}
          >
            <ArrowLeft />
            <span
              className={cn(
                "flex-1 text-center",
                SIDEBAR_LABEL_TRANSITION,
                sidebarLabelFade(isCollapsed),
              )}
            >
              {t("profile")}
            </span>
            <span
              aria-hidden="true"
              className={cn("size-4 shrink-0", sidebarLabelFade(isCollapsed))}
            />
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
      <SidebarGroupLabel className="pointer-events-none">
        {t("account")}
      </SidebarGroupLabel>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            isActive={isPersonalInfoActive}
            render={<Link href="/console/uc/profile" />}
            tooltip={t("personalInfo")}
          >
            <CircleUserRound />
            <span>{t("personalInfo")}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
        <SidebarMenuItem>
          <SidebarMenuButton
            isActive={isSessionsActive}
            render={<Link href="/console/uc/session" />}
            tooltip={t("sessions")}
          >
            <MonitorSmartphone />
            <span>{t("sessions")}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarGroup>
  );
}
