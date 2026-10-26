"use client";

import { Fragment, type ReactNode } from "react";

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

export type ConsoleSidebarSectionGroup = {
  label: string;
  items: ConsoleSidebarSectionItem[];
};

export type ConsoleSidebarBack =
  | { kind: "link"; href: string; onNavigate?: () => void }
  | { kind: "button"; onClick: () => void };

export type ConsoleSidebarSectionProps = {
  title: string;
  tooltip: string;
  back: ConsoleSidebarBack;
  groups: ConsoleSidebarSectionGroup[];
};

export function ConsoleSidebarSection({
  title,
  tooltip,
  back,
  groups,
}: ConsoleSidebarSectionProps) {
  const pathname = usePathname();
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";

  const backLink =
    back.kind === "link" ? (
      <Link href={back.href} onNavigate={back.onNavigate} draggable={false} />
    ) : undefined;
  const onBackClick = back.kind === "button" ? back.onClick : undefined;

  return (
    <SidebarGroup>
      <SidebarMenu>
        <SidebarMenuItem className="mb-2">
          <SidebarMenuButton
            render={backLink}
            onClick={onBackClick}
            tooltip={tooltip}
            className="select-none"
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
      {groups.map((group, index) => (
        <Fragment key={group.label}>
          <SidebarGroupLabel
            className={cn("pointer-events-none", index > 0 && "mt-4")}
          >
            {group.label}
          </SidebarGroupLabel>
          <SidebarMenu>
            {group.items.map((item) => (
              <SidebarMenuItem key={item.href} className={item.className}>
                <SidebarMenuButton
                  isActive={
                    pathname === item.href ||
                    pathname.startsWith(`${item.href}/`)
                  }
                  render={<Link href={item.href} draggable={false} />}
                  tooltip={item.label}
                  className="select-none"
                >
                  {item.icon}
                  <span>{item.label}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </Fragment>
      ))}
    </SidebarGroup>
  );
}
