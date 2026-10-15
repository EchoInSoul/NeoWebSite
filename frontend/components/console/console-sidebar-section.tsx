"use client";

import type { ReactNode } from "react";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft } from "lucide-react";
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

export type ConsoleSidebarSectionItem = {
  href: string;
  label: string;
  icon: ReactNode;
  className?: string;
};

export type ConsoleSidebarBack =
  { kind: "link"; href: string } | { kind: "button"; onClick: () => void };

export type ConsoleSidebarSectionProps = {
  title: string;
  tooltip: string;
  back: ConsoleSidebarBack;
  groupLabel: string;
  items: ConsoleSidebarSectionItem[];
};

export function ConsoleSidebarSection({
  title,
  tooltip,
  back,
  groupLabel,
  items,
}: ConsoleSidebarSectionProps) {
  const pathname = usePathname();
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";

  const backHref = back.kind === "link" ? back.href : undefined;
  const onBackClick = back.kind === "button" ? back.onClick : undefined;

  return (
    <SidebarGroup>
      <SidebarMenu>
        <SidebarMenuItem className="mb-2">
          <SidebarMenuButton
            render={backHref ? <Link href={backHref} /> : undefined}
            onClick={onBackClick}
            tooltip={tooltip}
          >
            <ArrowLeft />
            <span
              className={cn(
                "flex-1 text-center",
                SIDEBAR_LABEL_TRANSITION,
                sidebarLabelFade(isCollapsed),
              )}
            >
              {title}
            </span>
            <span
              aria-hidden="true"
              className={cn("size-4 shrink-0", sidebarLabelFade(isCollapsed))}
            />
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
      <SidebarGroupLabel className="pointer-events-none">
        {groupLabel}
      </SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => (
          <SidebarMenuItem key={item.href} className={item.className}>
            <SidebarMenuButton
              isActive={
                pathname === item.href || pathname.startsWith(`${item.href}/`)
              }
              render={<Link href={item.href} />}
              tooltip={item.label}
            >
              {item.icon}
              <span>{item.label}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}
