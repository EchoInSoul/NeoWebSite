"use client";

import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { useTranslations } from "next-intl";

import { useSidebar } from "@/components/ui/sidebar";

import { ConsoleUserMenu } from "./console-user-menu";

export function ConsoleSidebarFooter({
  displayName,
}: {
  displayName?: string;
}) {
  const { state, toggleSidebar } = useSidebar();
  const t = useTranslations("sidebar");
  const isCollapsed = state === "collapsed";
  const toggleLabel = isCollapsed ? t("expand") : t("collapse");

  return (
    <div className="flex w-full items-center">
      <div
        inert={isCollapsed ? true : undefined}
        className={
          "min-w-0 flex-1 transition-opacity " +
          (isCollapsed ? "opacity-0" : "opacity-100")
        }
      >
        <ConsoleUserMenu displayName={displayName} />
      </div>
      <button
        type="button"
        onClick={toggleSidebar}
        aria-label={toggleLabel}
        title={toggleLabel}
        aria-expanded={!isCollapsed}
        aria-controls="console-sidebar"
        className="flex size-9 shrink-0 items-center justify-center rounded-md text-neutral-900 hover:bg-neutral-200 hover:text-neutral-900 dark:text-neutral-100 dark:hover:bg-neutral-800 dark:hover:text-neutral-100"
      >
        {isCollapsed ? (
          <PanelLeftOpen size={16} />
        ) : (
          <PanelLeftClose size={16} />
        )}
      </button>
    </div>
  );
}
