"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import { syncThemeWithLocation, useThemeMode } from "@/lib/theme";

export function ThemeSync() {
  const pathname = usePathname();
  useThemeMode();

  useEffect(() => {
    syncThemeWithLocation();
  }, [pathname]);

  return null;
}
