"use client";

import { useSidebar } from "@/components/ui/sidebar";

export function ConsoleTopBanner() {
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";

  return (
    <header className="flex h-(--console-banner-h) w-full shrink-0 items-stretch border-b border-border bg-background">
      <div
        aria-hidden="true"
        className={
          "hidden shrink-0 border-r border-border transition-[width] duration-200 ease-linear md:block " +
          (isCollapsed ? "w-(--sidebar-width-icon)" : "w-(--sidebar-width)")
        }
      />
      <div className="flex min-w-0 flex-1 items-center px-4 md:px-6" />
    </header>
  );
}
