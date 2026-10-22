"use client";

import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import { PlusIcon, XIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  isValidEmailDomain,
  normalizeEmailDomain,
  parseEmailDomainList,
  serializeEmailDomainList,
} from "@/lib/register-settings";
import { cn } from "cn";

const EMAIL_DOMAIN_PRESETS: readonly string[] = [
  "qq.com",
  "outlook.com",
  "hotmail.com",
  "163.com",
  "icloud.com",
  "gmail.com",
];

const SUMMARY_DOMAIN_LIMIT = 3;

const fieldClassName =
  "flex min-h-9 w-full items-center gap-1 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm dark:bg-input/30";

const optionClassName =
  "flex w-full cursor-pointer items-center gap-2 rounded-md py-1 pr-1.5 pl-1.5 text-left text-sm/6 outline-hidden select-none";

export function AllowedEmailDomainsField({
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
  const containerRef = useRef<HTMLDivElement>(null);
  const fieldRef = useRef<HTMLDivElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [highlighted, setHighlighted] = useState<string | null>(null);
  const [position, setPosition] = useState<{
    top: number;
    left: number;
    width: number;
  } | null>(null);

  const domains = parseEmailDomainList(value);
  const normalizedQuery = normalizeEmailDomain(query);
  const validQuery =
    normalizedQuery !== "" && isValidEmailDomain(normalizedQuery);
  const alreadyListed =
    validQuery &&
    (domains.includes(normalizedQuery) ||
      EMAIL_DOMAIN_PRESETS.includes(normalizedQuery));
  const customCandidate = validQuery && !alreadyListed ? normalizedQuery : null;
  const customDomains = domains
    .filter((domain) => !EMAIL_DOMAIN_PRESETS.includes(domain))
    .sort((left, right) => left.localeCompare(right));
  const listedDomains = [...customDomains, ...EMAIL_DOMAIN_PRESETS];
  const options =
    customCandidate === null
      ? listedDomains
      : [customCandidate, ...listedDomains];
  const selectedDomains = listedDomains.filter((domain) =>
    domains.includes(domain),
  );
  const summaryPrefix = selectedDomains
    .slice(0, SUMMARY_DOMAIN_LIMIT)
    .join(", ");
  const summaryText =
    selectedDomains.length === 0
      ? null
      : selectedDomains.length > SUMMARY_DOMAIN_LIMIT
        ? `${summaryPrefix} …`
        : summaryPrefix;

  function setDomains(next: string[]) {
    onChange(serializeEmailDomainList(next));
  }

  function openPopup() {
    if (disabled) {
      return;
    }

    const rect = fieldRef.current?.getBoundingClientRect();

    if (rect === undefined) {
      return;
    }

    setPosition({
      top: rect.bottom + 4,
      left: rect.left,
      width: rect.width,
    });
    setOpen(true);
  }

  function commitTyped() {
    if (normalizedQuery === "") {
      return;
    }

    if (!isValidEmailDomain(normalizedQuery)) {
      setInvalid(true);
      return;
    }

    if (!domains.includes(normalizedQuery)) {
      setDomains([...domains, normalizedQuery]);
    }

    setQuery("");
    setInvalid(false);
    setHighlighted(null);
  }

  function toggleDomain(domain: string) {
    if (domains.includes(domain)) {
      setDomains(domains.filter((item) => item !== domain));
    } else {
      setDomains([...domains, domain]);
    }

    setQuery("");
    setInvalid(false);
    setHighlighted(null);

    if (!disabled) {
      openPopup();
      inputRef.current?.focus();
    }
  }

  function removeDomain(domain: string) {
    setDomains(domains.filter((item) => item !== domain));
    setInvalid(false);
    setHighlighted(null);
    inputRef.current?.focus();
  }

  function handleInputKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      openPopup();

      if (options.length === 0) {
        return;
      }

      const currentIndex =
        highlighted === null ? -1 : options.indexOf(highlighted);
      const nextIndex =
        event.key === "ArrowDown"
          ? (currentIndex + 1) % options.length
          : (currentIndex - 1 + options.length) % options.length;

      setHighlighted(options[nextIndex]);
      return;
    }

    if (event.key === "Enter") {
      event.preventDefault();

      if (highlighted !== null) {
        toggleDomain(highlighted);
        return;
      }

      commitTyped();
      return;
    }

    if (event.key === "," || event.key === "，") {
      event.preventDefault();
      commitTyped();
      return;
    }

    if (event.key === "Escape") {
      setOpen(false);
    }
  }

  useEffect(() => {
    if (!open) {
      return;
    }

    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node;

      if (
        containerRef.current?.contains(target) ||
        popupRef.current?.contains(target)
      ) {
        return;
      }

      setOpen(false);
    }

    function handleKeyDown(event: globalThis.KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }

    function handleScroll(event: Event) {
      const target = event.target as Node | null;

      if (target instanceof Node && popupRef.current?.contains(target)) {
        return;
      }

      setOpen(false);
    }

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    window.addEventListener("resize", handleScroll);
    window.addEventListener("scroll", handleScroll, true);

    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("resize", handleScroll);
      window.removeEventListener("scroll", handleScroll, true);
    };
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const rect = fieldRef.current?.getBoundingClientRect();

    if (rect === undefined) {
      return;
    }

    setPosition({
      top: rect.bottom + 4,
      left: rect.left,
      width: rect.width,
    });
  }, [open, domains.length, invalid]);

  const popup =
    open && position !== null ? (
      <div
        ref={popupRef}
        id={listId}
        role="listbox"
        aria-multiselectable="true"
        className="fixed z-50 max-h-70 overflow-y-auto rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10"
        style={position}
      >
        {options.map((option) => {
          const selected = domains.includes(option);
          const creatable = option === customCandidate;
          const deletable =
            !creatable && !EMAIL_DOMAIN_PRESETS.includes(option);
          const optionClassNameWithState = cn(
            optionClassName,
            highlighted === option
              ? "bg-accent text-accent-foreground"
              : "hover:bg-accent hover:text-accent-foreground",
          );

          if (creatable) {
            return (
              <button
                key={option}
                id={`${listId}-${option}`}
                role="option"
                aria-selected={false}
                className={optionClassNameWithState}
                onClick={() => toggleDomain(option)}
                onMouseDown={(event) => event.preventDefault()}
                onMouseEnter={() => setHighlighted(option)}
                type="button"
              >
                <PlusIcon className="size-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate">
                  {t("emailDomainAddCustom", { domain: option })}
                </span>
              </button>
            );
          }

          return (
            <div
              key={option}
              id={`${listId}-${option}`}
              role="option"
              aria-selected={selected}
              className={optionClassNameWithState}
              onClick={() => toggleDomain(option)}
              onMouseEnter={() => setHighlighted(option)}
            >
              <Checkbox
                aria-hidden="true"
                checked={selected}
                className="pointer-events-none"
                tabIndex={-1}
              />
              <span className="min-w-0 flex-1 truncate">{option}</span>
              {deletable ? (
                <Button
                  aria-label={t("emailDomainRemove", { domain: option })}
                  className="opacity-50 hover:opacity-100"
                  onClick={(event) => {
                    event.stopPropagation();
                    removeDomain(option);
                  }}
                  onMouseDown={(event) => event.preventDefault()}
                  size="icon-xs"
                  type="button"
                  variant="ghost"
                >
                  <XIcon className="size-3" />
                </Button>
              ) : null}
            </div>
          );
        })}
      </div>
    ) : null;

  return (
    <div
      ref={containerRef}
      className={disabled ? "pointer-events-none w-70 opacity-50" : "w-70"}
    >
      <div
        ref={fieldRef}
        className={cn(fieldClassName, invalid && "border-destructive")}
        onClick={() => {
          inputRef.current?.focus();
          openPopup();
        }}
      >
        {summaryText === null ? null : (
          <span
            aria-hidden="true"
            className="max-w-48 shrink-0 truncate text-foreground"
          >
            {summaryText}
          </span>
        )}
        <input
          ref={inputRef}
          aria-activedescendant={
            highlighted === null ? undefined : `${listId}-${highlighted}`
          }
          aria-autocomplete="list"
          aria-controls={listId}
          aria-expanded={open}
          aria-label={t("allowedEmailDomains")}
          autoComplete="off"
          className="min-w-8 flex-1 border-0 bg-transparent px-1 py-0.5 text-sm outline-hidden placeholder:text-muted-foreground"
          disabled={disabled}
          onBlur={() => setInvalid(false)}
          onClick={openPopup}
          onChange={(event) => {
            setQuery(event.currentTarget.value);
            setInvalid(false);
            setHighlighted(null);
            openPopup();
          }}
          onFocus={openPopup}
          onKeyDown={handleInputKeyDown}
          placeholder={
            domains.length === 0
              ? t("allowedEmailDomainsPlaceholder")
              : undefined
          }
          role="combobox"
          spellCheck={false}
          value={query}
        />
      </div>
      {invalid ? (
        <p className="mt-1 text-xs text-destructive">
          {t("emailDomainInvalid")}
        </p>
      ) : domains.length === 0 ? (
        <p className="mt-1 text-xs text-muted-foreground">
          {t("allowedEmailDomainsEmptyHint")}
        </p>
      ) : null}
      {popup === null ? null : createPortal(popup, document.body)}
    </div>
  );
}
