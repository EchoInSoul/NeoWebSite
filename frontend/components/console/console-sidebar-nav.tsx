"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { useTranslations } from "next-intl";
import { cn } from "cn";

import { useSidebar } from "@/components/ui/sidebar";
import {
  SidebarGroup,
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

  return (
    <SidebarGroup>
      <SidebarMenu>
        <SidebarMenuItem>
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
    </SidebarGroup>
  );
}
