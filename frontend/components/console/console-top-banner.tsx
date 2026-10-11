"use client";

import { Menu } from "lucide-react";
import { useTranslations } from "next-intl";

import { useSidebar } from "@/components/ui/sidebar";

export function ConsoleTopBanner() {
  const { state, openMobile, toggleSidebar } = useSidebar();
  const t = useTranslations("sidebar");
  const isCollapsed = state === "collapsed";

  return (
    <header className="flex h-(--console-banner-h) w-full shrink-0 items-stretch border-b border-border bg-background">
      <div className="flex shrink-0 items-center pl-1 min-[1200px]:hidden">
        <button
          type="button"
          onClick={toggleSidebar}
          aria-label={t("open")}
          aria-expanded={openMobile}
          className="flex size-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <Menu size={18} />
        </button>
      </div>
      <div
        aria-hidden="true"
        className={
          "hidden shrink-0 border-r border-border transition-[width] duration-200 ease-linear min-[1200px]:block " +
          (isCollapsed ? "w-(--sidebar-width-icon)" : "w-(--sidebar-width)")
        }
      />
      <div className="flex min-w-0 flex-1 items-center px-4 min-[1200px]:px-6" />
    </header>
  );
}
