"use client";

import { Fragment, type ReactNode } from "react";
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronRight } from "lucide-react";
import { cn } from "cn";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";

export type ConsoleListSortDirection = "asc" | "desc";

export type ConsoleListSelectAllProperties = {
  isAllSelected: boolean;
  isSomeSelected: boolean;
  onToggleAllSelected: (checked: boolean) => void;
  selectAllLabel: string;
};

export type ConsoleListColumn<TRow> = {
  key: string;
  header?: ReactNode;
  sortField?: string;
  headerClassName?: string;
  cellClassName?: string;
  render: (row: TRow) => ReactNode;
};

type ConsoleListTableExpansionProperties<TRow> =
  | {
      expandedKeys?: ReadonlySet<string>;
      onToggleRowExpanded: (rowKey: string) => void;
      getRowExpandLabel: (row: TRow) => string;
      renderExpandedContent: (row: TRow) => ReactNode;
    }
  | {
      expandedKeys?: undefined;
      onToggleRowExpanded?: undefined;
      getRowExpandLabel?: undefined;
      renderExpandedContent?: undefined;
    };

export type ConsoleListTableProperties<TRow> = {
  rows: readonly TRow[];
  columns: readonly ConsoleListColumn<TRow>[];
  getRowKey: (row: TRow) => string;
  selectAll?: ConsoleListSelectAllProperties;
  sortField?: string;
  sortDirection?: ConsoleListSortDirection;
  onToggleSort?: (sortField: string) => void;
  selectedKeys?: ReadonlySet<string>;
  onToggleRowSelected?: (rowKey: string, checked: boolean) => void;
  getRowSelectionLabel?: (row: TRow) => string;
  getRowSelectionDisabled?: (row: TRow) => boolean;
  rowActions?: (row: TRow) => ReactNode;
  actionsHeader?: ReactNode;
  getRowClassName?: (row: TRow, rowIndex: number) => string | undefined;
  isBusy?: boolean;
  emptyState?: ReactNode;
  className?: string;
} & ConsoleListTableExpansionProperties<TRow>;

export function ConsoleListTable<TRow>({
  rows,
  columns,
  getRowKey,
  selectAll,
  sortField,
  sortDirection = "desc",
  onToggleSort,
  selectedKeys,
  onToggleRowSelected,
  getRowSelectionLabel,
  getRowSelectionDisabled,
  expandedKeys,
  onToggleRowExpanded,
  getRowExpandLabel,
  renderExpandedContent,
  rowActions,
  actionsHeader,
  getRowClassName,
  isBusy = false,
  emptyState,
  className,
}: ConsoleListTableProperties<TRow>) {
  const isExpandable =
    onToggleRowExpanded !== undefined && renderExpandedContent !== undefined;
  const shouldRenderHeader =
    columns.some((column) => column.header !== undefined) ||
    actionsHeader !== undefined;
  const totalCellCount =
    (isExpandable ? 1 : 0) +
    (onToggleRowSelected === undefined ? 0 : 1) +
    columns.length +
    (rowActions === undefined ? 0 : 1);

  if (rows.length === 0) {
    return (
      <div className="overflow-hidden rounded-[8px] border-t border-border bg-card">
        {emptyState}
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-[8px] border-t border-border bg-card">
      <table
        className={cn(
          "w-full border-separate border-spacing-0 text-left text-sm text-card-foreground",
          className,
        )}
      >
        {shouldRenderHeader ? (
          <thead>
            <tr>
              {isExpandable ? (
                <th className="h-[46px] w-10 border-b border-border pl-3" />
              ) : null}
              {onToggleRowSelected === undefined ? null : (
                <th className="h-[46px] w-11 border-b border-border pr-0 pl-4 text-left align-middle">
                  {selectAll === undefined ? null : (
                    <Checkbox
                      checked={selectAll.isAllSelected}
                      indeterminate={selectAll.isSomeSelected}
                      onCheckedChange={selectAll.onToggleAllSelected}
                      disabled={isBusy}
                      aria-label={selectAll.selectAllLabel}
                      className="data-checked:border-blue-600 data-checked:bg-blue-600 data-checked:text-white dark:data-checked:border-blue-500 dark:data-checked:bg-blue-500"
                    />
                  )}
                </th>
              )}
              {columns.map((column) => {
                const isSortActive =
                  onToggleSort !== undefined &&
                  column.sortField !== undefined &&
                  column.sortField === sortField;
                return (
                  <th
                    key={column.key}
                    className={cn(
                      "h-[46px] border-b border-border px-3 text-left align-middle text-xs leading-4 font-semibold whitespace-nowrap text-foreground",
                      column.headerClassName,
                    )}
                    aria-sort={
                      isSortActive
                        ? sortDirection === "asc"
                          ? "ascending"
                          : "descending"
                        : undefined
                    }
                  >
                    {onToggleSort !== undefined &&
                    column.sortField !== undefined ? (
                      <ConsoleListSortHeader
                        label={column.header}
                        isActive={isSortActive}
                        isAscending={sortDirection === "asc"}
                        onToggle={() =>
                          onToggleSort(column.sortField ?? column.key)
                        }
                      />
                    ) : (
                      (column.header ?? null)
                    )}
                  </th>
                );
              })}
              {rowActions === undefined ? null : (
                <th className="h-[46px] w-[60px] border-b border-border pr-4 pl-3 text-right align-middle text-xs leading-4 font-semibold whitespace-nowrap text-foreground">
                  {actionsHeader ?? null}
                </th>
              )}
            </tr>
          </thead>
        ) : null}
        <tbody>
          {rows.map((row, rowIndex) => {
            const rowKey = getRowKey(row);
            const isSelected = selectedKeys?.has(rowKey) ?? false;
            const isExpanded = expandedKeys?.has(rowKey) ?? false;
            return (
              <Fragment key={rowKey}>
                <tr
                  data-state={isSelected ? "selected" : undefined}
                  className={cn(
                    "h-[59px] even:bg-muted/60 data-[state=selected]:bg-muted",
                    getRowClassName?.(row, rowIndex),
                  )}
                >
                  {isExpandable ? (
                    <td className="w-10 pl-3 align-middle">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label={getRowExpandLabel?.(row)}
                        aria-expanded={isExpanded}
                        onClick={() => onToggleRowExpanded?.(rowKey)}
                      >
                        <ChevronRight
                          aria-hidden="true"
                          className={cn(
                            "transition-transform",
                            isExpanded && "rotate-90",
                          )}
                        />
                      </Button>
                    </td>
                  ) : null}
                  {onToggleRowSelected === undefined ? null : (
                    <td className="pr-0 pl-4 align-middle">
                      <Checkbox
                        checked={isSelected}
                        onCheckedChange={(checkedValue) =>
                          onToggleRowSelected(rowKey, checkedValue)
                        }
                        disabled={
                          isBusy || getRowSelectionDisabled?.(row) === true
                        }
                        aria-label={getRowSelectionLabel?.(row)}
                        className="data-checked:border-blue-600 data-checked:bg-blue-600 data-checked:text-white dark:data-checked:border-blue-500 dark:data-checked:bg-blue-500"
                      />
                    </td>
                  )}
                  {columns.map((column) => (
                    <td
                      key={column.key}
                      className={cn("px-3 align-middle", column.cellClassName)}
                    >
                      {column.render(row)}
                    </td>
                  ))}
                  {rowActions === undefined ? null : (
                    <td className="pr-4 pl-3 text-right align-middle">
                      <span className="flex items-center justify-end">
                        {rowActions(row)}
                      </span>
                    </td>
                  )}
                </tr>
                {isExpandable && isExpanded ? (
                  <tr className="bg-muted/40">
                    <td colSpan={totalCellCount} className="px-4 pt-1 pb-4">
                      {renderExpandedContent?.(row)}
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export type ConsoleListSortHeaderProperties = {
  label: ReactNode;
  isActive?: boolean;
  isAscending?: boolean;
  ariaLabel?: string;
  onToggle: () => void;
  className?: string;
};

export function ConsoleListSortHeader({
  label,
  isActive = false,
  isAscending = false,
  ariaLabel,
  onToggle,
  className,
}: ConsoleListSortHeaderProperties) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-label={ariaLabel}
      className={cn(
        "group/sort -mx-1 inline-flex items-center gap-1 rounded-[4px] px-1 py-0.5 whitespace-nowrap",
        className,
      )}
    >
      {label}
      {isActive ? (
        isAscending ? (
          <ArrowUp aria-hidden="true" className="size-3.5" />
        ) : (
          <ArrowDown aria-hidden="true" className="size-3.5" />
        )
      ) : (
        <ArrowUpDown
          aria-hidden="true"
          className="size-3.5 opacity-0 transition-opacity group-hover/sort:opacity-50"
        />
      )}
    </button>
  );
}

export type ConsoleListTitleBlockProperties = {
  title: ReactNode;
  trailing?: ReactNode;
  secondary?: string;
  className?: string;
};

export function ConsoleListTitleBlock({
  title,
  trailing,
  secondary,
  className,
}: ConsoleListTitleBlockProperties) {
  return (
    <span className={cn("block min-w-0", className)}>
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="block max-w-72 truncate text-sm font-medium">
          {title}
        </span>
        {trailing === undefined ? null : trailing}
      </span>
      {secondary === undefined ? null : (
        <span className="block truncate font-mono text-xs text-muted-foreground">
          {secondary}
        </span>
      )}
    </span>
  );
}
