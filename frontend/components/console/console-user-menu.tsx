"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  ExternalLink,
  Languages,
  LogOut,
  Monitor,
  Moon,
  Sun,
  UserRound,
} from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarMenuButton } from "@/components/ui/sidebar";
import { useSignout } from "@/hooks/use-signout";
import { SUPPORTED_LOCALES, type Locale } from "@/i18n/config";
import { setUserLocale } from "@/i18n/set-locale";
import { useThemeMode, type ThemeMode } from "@/lib/theme";

const themeModeIcons = {
  system: Monitor,
  light: Sun,
  dark: Moon,
} as const;

const themeModeOrder: readonly ThemeMode[] = ["system", "light", "dark"];

const localeLabels = {
  "zh-CN": "localeZhCn",
  en: "localeEn",
} as const;

export function ConsoleUserMenu({ displayName }: { displayName?: string }) {
  const t = useTranslations("userMenu");
  const locale = useLocale();
  const router = useRouter();
  const [isSwitchingLocale, startLocaleSwitch] = useTransition();
  const { themeMode, setThemeMode } = useThemeMode();
  const { isSigningOut, signOut } = useSignout();
  const [isSignOutConfirmOpen, setIsSignOutConfirmOpen] = useState(false);

  const themeModeLabels: Record<ThemeMode, string> = {
    system: t("themeSystem"),
    light: t("themeLight"),
    dark: t("themeDark"),
  };

  function handleThemeModeChange(nextThemeMode: string) {
    setThemeMode(nextThemeMode as ThemeMode);
  }

  function handleLocaleChange(nextLocale: string) {
    startLocaleSwitch(async () => {
      await setUserLocale(nextLocale);
      router.refresh();
    });
  }

  async function handleConfirmSignOut() {
    await signOut();
    setIsSignOutConfirmOpen(false);
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<SidebarMenuButton className="pr-2" />}
          aria-label={t("openMenu")}
        >
          <UserRound />
          {displayName ? <span className="truncate">{displayName}</span> : null}
        </DropdownMenuTrigger>
        <DropdownMenuContent side="top" align="start" className="w-52">
          <DropdownMenuItem
            render={<a href="/" target="_blank" rel="noreferrer" />}
          >
            <ExternalLink />
            <span>{t("visitSite")}</span>
          </DropdownMenuItem>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <Sun />
              <span>{t("theme")}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuRadioGroup
                value={themeMode}
                onValueChange={handleThemeModeChange}
              >
                {themeModeOrder.map((mode) => {
                  const ThemeModeIcon = themeModeIcons[mode];
                  return (
                    <DropdownMenuRadioItem key={mode} value={mode}>
                      <ThemeModeIcon />
                      <span>{themeModeLabels[mode]}</span>
                    </DropdownMenuRadioItem>
                  );
                })}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <Languages />
              <span>{t("language")}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuRadioGroup
                value={locale}
                onValueChange={handleLocaleChange}
              >
                {SUPPORTED_LOCALES.map((availableLocale: Locale) => (
                  <DropdownMenuRadioItem
                    key={availableLocale}
                    value={availableLocale}
                    disabled={isSwitchingLocale}
                  >
                    <span>{t(localeLabels[availableLocale])}</span>
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSeparator />

          <DropdownMenuItem
            disabled={isSigningOut}
            onClick={() => setIsSignOutConfirmOpen(true)}
          >
            <LogOut />
            <span>{t("signOut")}</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog
        open={isSignOutConfirmOpen}
        onOpenChange={setIsSignOutConfirmOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <LogOut />
            </AlertDialogMedia>
            <AlertDialogTitle>{t("signOutConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("signOutConfirmDescription")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isSigningOut}>
              {t("signOutConfirmCancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={isSigningOut}
              onClick={handleConfirmSignOut}
            >
              {t("signOutConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
