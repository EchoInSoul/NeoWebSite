"use client";

import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ChevronDownIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "cn";

const COMPLEXITY_CODES = [
  "digits",
  "lowercase",
  "uppercase",
  "symbols",
] as const;

type ComplexityCode = (typeof COMPLEXITY_CODES)[number];

export function PasswordComplexityField({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const t = useTranslations("settings");
  const listId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<{
    top: number;
    left: number;
    width: number;
  } | null>(null);

  const labels: Record<ComplexityCode, string> = {
    digits: t("passwordComplexityDigits"),
    lowercase: t("passwordComplexityLowercase"),
    uppercase: t("passwordComplexityUppercase"),
    symbols: t("passwordComplexitySymbols"),
  };
  const selected = COMPLEXITY_CODES.filter((code) =>
    value.split(",").includes(code),
  );
  const summary = selected.map((code) => labels[code]).join(", ");

  function placePopup() {
    const rect = triggerRef.current?.getBoundingClientRect();

    if (rect === undefined) {
      return;
    }

    setPosition({
      top: rect.bottom + 4,
      left: rect.left,
      width: rect.width,
    });
  }

  function toggleOpen() {
    if (disabled) {
      return;
    }

    if (!open) {
      placePopup();
    }

    setOpen(!open);
  }

  function toggleCode(code: ComplexityCode) {
    const next = selected.includes(code)
      ? selected.filter((item) => item !== code)
      : [...selected, code];

    onChange(COMPLEXITY_CODES.filter((item) => next.includes(item)).join(","));
  }

  useEffect(() => {
    if (!open) {
      return;
    }

    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node;

      if (
        triggerRef.current?.contains(target) ||
        popupRef.current?.contains(target)
      ) {
        return;
      }

      setOpen(false);
    }

    function handleClose() {
      setOpen(false);
    }

    document.addEventListener("pointerdown", handlePointerDown);
    window.addEventListener("resize", handleClose);
    window.addEventListener("scroll", handleClose, true);

    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      window.removeEventListener("resize", handleClose);
      window.removeEventListener("scroll", handleClose, true);
    };
  }, [open]);

  const popup =
    open && position !== null ? (
      <div
        ref={popupRef}
        id={listId}
        role="listbox"
        aria-multiselectable="true"
        className="fixed z-50 min-w-36 rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10"
        style={position}
      >
        {COMPLEXITY_CODES.map((code) => {
          const checked = selected.includes(code);

          return (
            <div
              key={code}
              role="option"
              aria-selected={checked}
              className="flex cursor-pointer items-center gap-2 rounded-md py-1 pr-2 pl-1.5 text-sm/6 outline-hidden select-none hover:bg-accent hover:text-accent-foreground"
              onClick={() => toggleCode(code)}
            >
              <Checkbox
                aria-hidden="true"
                checked={checked}
                className="pointer-events-none"
                tabIndex={-1}
              />
              <span className="min-w-0 flex-1 truncate">{labels[code]}</span>
            </div>
          );
        })}
      </div>
    ) : null;

  return (
    <>
      <button
        ref={triggerRef}
        aria-controls={listId}
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-label={t("passwordComplexity")}
        className={cn(
          "flex h-9 w-70 items-center justify-between gap-1.5 rounded-lg border border-input bg-transparent pr-2 pl-2.5 text-sm whitespace-nowrap shadow-xs outline-none select-none focus-visible:border-ring dark:bg-input/30 dark:hover:bg-input/50",
          disabled && "cursor-not-allowed opacity-50",
        )}
        disabled={disabled}
        onClick={toggleOpen}
        type="button"
      >
        <span
          className={cn(
            "min-w-0 flex-1 truncate text-left",
            summary === "" && "text-muted-foreground",
          )}
        >
          {summary === "" ? t("passwordComplexityPlaceholder") : summary}
        </span>
        <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" />
      </button>
      {popup === null ? null : createPortal(popup, document.body)}
    </>
  );
}
