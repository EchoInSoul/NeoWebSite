"use client";

import { useCallback, useSyncExternalStore } from "react";

import { CHANGE_EVENT_NAME, THEME_STORAGE_KEY } from "@/lib/bootstrap-scripts";

export type ThemeMode = "system" | "light" | "dark";

const DARK_COLOR_SCHEME_QUERY = "(prefers-color-scheme: dark)";

const THEME_MODES: readonly ThemeMode[] = ["system", "light", "dark"];

function isThemeMode(value: string | null): value is ThemeMode {
  return value === "system" || value === "light" || value === "dark";
}

function subscribe(onStoreChange: () => void): () => void {
  const darkColorScheme = window.matchMedia(DARK_COLOR_SCHEME_QUERY);
  darkColorScheme.addEventListener("change", onStoreChange);
  window.addEventListener("storage", onStoreChange);
  window.addEventListener(CHANGE_EVENT_NAME, onStoreChange);
  return () => {
    darkColorScheme.removeEventListener("change", onStoreChange);
    window.removeEventListener("storage", onStoreChange);
    window.removeEventListener(CHANGE_EVENT_NAME, onStoreChange);
  };
}

function getSnapshot(): ThemeMode {
  try {
    const storedThemeMode = window.localStorage.getItem(THEME_STORAGE_KEY);
    return isThemeMode(storedThemeMode) ? storedThemeMode : "system";
  } catch {
    // localStorage 不可用时按默认走
    return "system";
  }
}

function getServerSnapshot(): ThemeMode {
  return "system";
}

function applyThemeMode(themeMode: ThemeMode): void {
  const isDark =
    themeMode === "dark" ||
    (themeMode === "system" &&
      window.matchMedia(DARK_COLOR_SCHEME_QUERY).matches);
  const rootElement = document.documentElement;
  rootElement.classList.add("disable-theme-transitions");
  rootElement.classList.toggle("dark", isDark);
  void rootElement.offsetHeight;
  rootElement.classList.remove("disable-theme-transitions");
}

export function useThemeMode() {
  const themeMode = useSyncExternalStore(
    subscribe,
    getSnapshot,
    getServerSnapshot,
  );

  const setThemeMode = useCallback((nextThemeMode: ThemeMode) => {
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, nextThemeMode);
    } catch {
      // localStorage 不可用时仅作用于当前会话
    }
    applyThemeMode(nextThemeMode);
    window.dispatchEvent(new Event(CHANGE_EVENT_NAME));
  }, []);

  return { themeMode, setThemeMode, themeModes: THEME_MODES };
}
