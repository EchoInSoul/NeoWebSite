"use client";

import { Fragment, type ReactNode } from "react";
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronRight } from "lucide-react";
import { cn } from "cn";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export type ConsoleListSortDirection = "asc" | "desc";

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
  density?: "default" | "dense";
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
  density = "default",
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
    return <>{emptyState ?? null}</>;
  }

  return (
    <Table
      className={cn(
        density === "dense" && "text-[13px] [&_td]:py-1.5 [&_th]:h-8",
        className,
      )}
    >
      {shouldRenderHeader ? (
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {isExpandable ? <TableHead className="w-[40px] px-1.5" /> : null}
            {onToggleRowSelected === undefined ? null : (
              <TableHead className="w-[48px] px-4" />
            )}
            {columns.map((column) => {
              const isSortActive =
                onToggleSort !== undefined &&
                column.sortField !== undefined &&
                column.sortField === sortField;
              return (
                <TableHead
                  key={column.key}
                  className={column.headerClassName}
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
                </TableHead>
              );
            })}
            {rowActions === undefined ? null : (
              <TableHead className="w-[56px] pr-4">
                {actionsHeader ?? null}
              </TableHead>
            )}
          </TableRow>
        </TableHeader>
      ) : null}
      <TableBody>
        {rows.map((row, rowIndex) => {
          const rowKey = getRowKey(row);
          const isSelected = selectedKeys?.has(rowKey) ?? false;
          const isExpanded = expandedKeys?.has(rowKey) ?? false;
          return (
            <Fragment key={rowKey}>
              <TableRow
                data-state={isSelected ? "selected" : undefined}
                className={getRowClassName?.(row, rowIndex)}
              >
                {isExpandable ? (
                  <TableCell className="w-[40px] px-1.5">
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
                  </TableCell>
                ) : null}
                {onToggleRowSelected === undefined ? null : (
                  <TableCell className="w-[48px] px-4">
                    <Checkbox
                      checked={isSelected}
                      onCheckedChange={(checkedValue) =>
                        onToggleRowSelected(rowKey, checkedValue)
                      }
                      disabled={
                        isBusy || getRowSelectionDisabled?.(row) === true
                      }
                      aria-label={getRowSelectionLabel?.(row)}
                    />
                  </TableCell>
                )}
                {columns.map((column) => (
                  <TableCell key={column.key} className={column.cellClassName}>
                    {column.render(row)}
                  </TableCell>
                ))}
                {rowActions === undefined ? null : (
                  <TableCell className="w-[56px] pr-4">
                    <span className="flex items-center justify-end">
                      {rowActions(row)}
                    </span>
                  </TableCell>
                )}
              </TableRow>
              {isExpandable && isExpanded ? (
                <TableRow className="hover:bg-transparent">
                  <TableCell
                    colSpan={totalCellCount}
                    className="px-4 pt-1 pb-4"
                  >
                    {renderExpandedContent?.(row)}
                  </TableCell>
                </TableRow>
              ) : null}
            </Fragment>
          );
        })}
      </TableBody>
    </Table>
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
