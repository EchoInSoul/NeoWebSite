"use client";

import { useState } from "react";
import {
  AlignLeft,
  CirclePlus,
  Globe,
  SearchX,
  Tags,
  TextQuote,
  Type,
  X,
} from "lucide-react";
import { cn } from "cn";
import { useTranslations } from "next-intl";

import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

const inputClassName = "w-70";
const textareaClassName = "w-70 min-h-20";

const thumbnailBoxClassName =
  "flex h-16 w-16 items-center justify-center overflow-hidden rounded-lg border border-dashed border-muted-foreground/40 bg-muted/40";

const browserMockClassName =
  "relative isolate flex h-[88px] w-[350px] max-w-full shrink-0 items-start overflow-hidden rounded border border-[#8c8f94] bg-neutral-50 py-0 pl-4 pr-0 pt-4 dark:border-neutral-700 dark:bg-neutral-900";

const browserMockGridClassName =
  "z-[1] grid h-full w-full shrink-0 grid-cols-[58px_minmax(0,1fr)] gap-4";

const largePreviewClassName =
  "flex h-[58px] w-[58px] shrink-0 items-center justify-center rounded-[10px] border border-dashed border-muted-foreground/40 bg-muted/40 text-muted-foreground dark:border-neutral-600 dark:bg-neutral-800/60";

const browserTabClassName =
  "z-[1] flex min-w-0 flex-1 items-start gap-4 self-stretch rounded-tl-[10px] border-l border-t border-black/10 bg-gradient-to-b from-neutral-200 to-neutral-100 py-1 pl-3 pr-1 shadow-[0_10px_22px_0_rgba(0,0,0,0.08)] dark:border-white/20 dark:from-[#2c3338] dark:to-[#111111] dark:shadow-[0_10px_22px_0_rgba(0,0,0,0.2)]";

const browserTabBarClassName =
  "grid min-w-0 flex-1 grid-cols-[24px_minmax(0,1fr)_24px] items-center gap-2 rounded border border-black/10 bg-white/60 p-2 text-[13px] font-medium leading-5 text-neutral-600 dark:border-transparent dark:bg-[#3c434a] dark:text-[#f0f0f1] dark:shadow-[0_1px_3px_0_rgba(0,0,0,0.1)]";

const chooseImageButtonClassName =
  "rounded-md border border-blue-500 bg-neutral-50 px-4 py-1.5 text-sm text-blue-600 transition-colors hover:bg-blue-50 dark:border-blue-400 dark:bg-[#030303] dark:text-blue-400 dark:hover:bg-blue-950/40";

const removeImageButtonClassName =
  "rounded-md border border-red-500 bg-neutral-50 px-4 py-1.5 text-sm text-red-600 transition-colors hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-400 dark:bg-[#030303] dark:text-red-400 dark:hover:bg-red-950/40";

export function SiteSettingsForm() {
  const t = useTranslations("settings");
  const [title, setTitle] = useState("");
  const [subtitle, setSubtitle] = useState("");
  const [description, setDescription] = useState("");
  const [keywords, setKeywords] = useState("");
  const [url, setUrl] = useState("");
  const [blockSearchEngines, setBlockSearchEngines] = useState(false);

  return (
    <ConsoleSettingsGroup title={t("siteGroup")}>
      <ConsoleSettingsRow
        control={
          <Input
            aria-label={t("siteTitle")}
            className={inputClassName}
            onChange={(event) => setTitle(event.target.value)}
            value={title}
          />
        }
        icon={<Type />}
        label={t("siteTitle")}
      />
      <ConsoleSettingsRow
        control={
          <Input
            aria-label={t("siteSubtitle")}
            className={inputClassName}
            onChange={(event) => setSubtitle(event.target.value)}
            value={subtitle}
          />
        }
        icon={<TextQuote />}
        label={t("siteSubtitle")}
      />
      <ConsoleSettingsRow
        className="items-start"
        control={
          <button
            type="button"
            aria-label={t("siteLogo")}
            disabled
            className={cn(thumbnailBoxClassName, "cursor-not-allowed")}
          >
            <CirclePlus
              size={20}
              strokeWidth={1.5}
              aria-hidden="true"
              className="text-muted-foreground"
            />
          </button>
        }
        icon={<CirclePlus />}
        label={t("siteLogo")}
      />
      <ConsoleSettingsRow
        className="items-start"
        control={
          <div className="flex flex-col gap-1.5">
            <div
              role="group"
              aria-label={t("faviconPreview")}
              title={t("faviconPreview")}
              className={browserMockClassName}
            >
              <div className={browserMockGridClassName}>
                <span
                  role="img"
                  aria-label={t("faviconLargePreviewAlt")}
                  title={t("faviconLargePreviewAlt")}
                  className={largePreviewClassName}
                >
                  <CirclePlus size={20} strokeWidth={1.5} aria-hidden="true" />
                </span>
                <div className={browserTabClassName}>
                  <span
                    aria-hidden="true"
                    className="flex h-10 w-12 shrink-0 items-center gap-1.5"
                  >
                    <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
                    <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
                    <span className="h-3 w-3 rounded-full bg-[#28c840]" />
                  </span>
                  <span className={browserTabBarClassName}>
                    <span
                      role="img"
                      aria-label={t("faviconTabIconAlt")}
                      title={t("faviconTabIconAlt")}
                      className="h-6 w-6 shrink-0 rounded-[2px] bg-neutral-300 dark:bg-white/20"
                    />
                    {title.trim() !== "" ? (
                      <span className="truncate select-text">
                        {title.trim()}
                      </span>
                    ) : (
                      <span aria-hidden="true" />
                    )}
                    <span
                      role="img"
                      aria-label={t("browserTabClose")}
                      title={t("browserTabClose")}
                      className="flex h-6 w-6 shrink-0 items-center justify-center text-neutral-500 dark:text-[#f0f0f1]"
                    >
                      <X size={14} aria-hidden="true" />
                    </span>
                  </span>
                </div>
              </div>
            </div>
            <div className="mt-2 flex items-center gap-3">
              <Button
                type="button"
                disabled
                variant="outline"
                className={chooseImageButtonClassName}
              >
                {t("changeIcon")}
              </Button>
              <Button
                type="button"
                disabled
                variant="outline"
                className={removeImageButtonClassName}
              >
                {t("removeIcon")}
              </Button>
            </div>
          </div>
        }
        icon={<CirclePlus />}
        label={t("siteIcon")}
      />
      <ConsoleSettingsRow
        className="items-start"
        control={
          <Textarea
            aria-label={t("siteDescription")}
            className={textareaClassName}
            onChange={(event) => setDescription(event.target.value)}
            value={description}
          />
        }
        icon={<AlignLeft />}
        label={t("siteDescription")}
      />
      <ConsoleSettingsRow
        control={
          <Input
            aria-label={t("siteKeywords")}
            className={inputClassName}
            onChange={(event) => setKeywords(event.target.value)}
            value={keywords}
          />
        }
        icon={<Tags />}
        label={t("siteKeywords")}
      />
      <ConsoleSettingsRow
        control={
          <Input
            aria-label={t("siteUrl")}
            className={inputClassName}
            onChange={(event) => setUrl(event.target.value)}
            value={url}
          />
        }
        icon={<Globe />}
        label={t("siteUrl")}
      />
      <ConsoleSettingsRow
        control={
          <Switch
            aria-label={t("blockSearchEngines")}
            checked={blockSearchEngines}
            disabled
            onCheckedChange={setBlockSearchEngines}
          />
        }
        icon={<SearchX />}
        label={t("blockSearchEngines")}
      />
    </ConsoleSettingsGroup>
  );
}
