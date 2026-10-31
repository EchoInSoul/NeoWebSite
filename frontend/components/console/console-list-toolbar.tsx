"use client";

import type { ReactNode } from "react";
import { ArrowDown, ArrowUp, RefreshCw, Search } from "lucide-react";
import { cn } from "cn";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export type ConsoleListToolbarProperties = {
  start?: ReactNode;
  end?: ReactNode;
  isSelectionActive?: boolean;
  selectionActions?: ReactNode;
  isBusy?: boolean;
  isRefreshing?: boolean;
  onRefresh?: () => void;
  refreshLabel: string;
  className?: string;
};

export function ConsoleListToolbar({
  start,
  end,
  isSelectionActive = false,
  selectionActions,
  isBusy = false,
  isRefreshing = false,
  onRefresh,
  refreshLabel,
  className,
}: ConsoleListToolbarProperties) {
  return (
    <div
      className={cn(
        "flex h-[46px] items-center justify-between gap-3 px-4 text-sm select-none max-md:h-auto max-md:flex-col max-md:items-stretch max-md:gap-2 max-md:py-2.5",
        className,
      )}
    >
      <span className="flex min-w-0 flex-1 items-center gap-2">
        {isSelectionActive ? null : start}
      </span>
      <span className="flex shrink-0 items-center gap-2">
        {isSelectionActive ? selectionActions : end}
        {onRefresh === undefined ? null : (
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={onRefresh}
            disabled={isBusy || isRefreshing}
            aria-label={refreshLabel}
            title={refreshLabel}
          >
            <RefreshCw aria-hidden="true" />
          </Button>
        )}
      </span>
    </div>
  );
}

export type ConsoleListSearchInputProperties = {
  value: string;
  onValueChange: (nextValue: string) => void;
  placeholder: string;
  disabled?: boolean;
  className?: string;
};

export function ConsoleListSearchInput({
  value,
  onValueChange,
  placeholder,
  disabled = false,
  className,
}: ConsoleListSearchInputProperties) {
  return (
    <span className={cn("relative w-full max-w-48", className)}>
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        type="search"
        value={value}
        onChange={(changeEvent) => onValueChange(changeEvent.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        disabled={disabled}
        className="bg-background pl-9"
      />
    </span>
  );
}

export type ConsoleListSortButtonProperties = {
  label: string;
  isAscending: boolean;
  onToggleOrder: () => void;
  hint?: string;
  disabled?: boolean;
};

export function ConsoleListSortButton({
  label,
  isAscending,
  onToggleOrder,
  hint,
  disabled = false,
}: ConsoleListSortButtonProperties) {
  return (
    <Button
      type="button"
      variant="outline"
      onClick={onToggleOrder}
      disabled={disabled}
      aria-label={hint ?? label}
      title={hint ?? label}
    >
      {label}
      {isAscending ? (
        <ArrowUp aria-hidden="true" />
      ) : (
        <ArrowDown aria-hidden="true" />
      )}
    </Button>
  );
}
