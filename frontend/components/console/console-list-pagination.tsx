"use client";

import { Fragment, type ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { cn } from "cn";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

function buildPageOptionValues(
  totalPages: number,
  currentPage: number,
): readonly number[] {
  const optionValues = new Set<number>([
    1,
    totalPages,
    currentPage - 1,
    currentPage,
    currentPage + 1,
  ]);
  return Array.from(optionValues)
    .filter((optionValue) => optionValue >= 1 && optionValue <= totalPages)
    .sort((leftValue, rightValue) => leftValue - rightValue);
}

export type ConsoleListPaginationProperties = {
  currentPage: number;
  totalPages: number;
  pageSize: number;
  pageSizeOptions: readonly number[];
  onPageChange: (nextPage: number) => void;
  onPageSizeChange: (nextPageSize: number) => void;
  summary: ReactNode;
  previousLabel: string;
  nextLabel: string;
  pageLabel: string;
  perPageLabel: string;
  pageSelectAriaLabel: string;
  pageSizeSelectAriaLabel: string;
  className?: string;
};

export function ConsoleListPagination({
  currentPage,
  totalPages,
  pageSize,
  pageSizeOptions,
  onPageChange,
  onPageSizeChange,
  summary,
  previousLabel,
  nextLabel,
  pageLabel,
  perPageLabel,
  pageSelectAriaLabel,
  pageSizeSelectAriaLabel,
  className,
}: ConsoleListPaginationProperties) {
  const safeTotalPages = Math.max(1, totalPages);
  const safeCurrentPage = Math.min(Math.max(1, currentPage), safeTotalPages);
  const pageOptionValues = buildPageOptionValues(
    safeTotalPages,
    safeCurrentPage,
  );
  const isPreviousDisabled = safeCurrentPage <= 1;
  const isNextDisabled = safeCurrentPage >= safeTotalPages;

  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-3 border-t border-border px-4 py-3 text-sm text-muted-foreground",
        className,
      )}
    >
      <p>{summary}</p>
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          disabled={isPreviousDisabled}
          aria-label={previousLabel}
          title={previousLabel}
          onClick={() => onPageChange(safeCurrentPage - 1)}
        >
          <ChevronLeft aria-hidden="true" />
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          disabled={isNextDisabled}
          aria-label={nextLabel}
          title={nextLabel}
          onClick={() => onPageChange(safeCurrentPage + 1)}
        >
          <ChevronRight aria-hidden="true" />
        </Button>
        <Select
          value={safeCurrentPage}
          items={pageOptionValues.map((pageOption) => ({
            value: pageOption,
            label: String(pageOption),
          }))}
          onValueChange={(nextValue) => {
            if (typeof nextValue === "number") {
              onPageChange(nextValue);
            }
          }}
        >
          <SelectTrigger
            size="sm"
            aria-label={pageSelectAriaLabel}
            className="h-8 min-w-[72px] gap-1 px-2 text-[13px]"
          >
            <SelectValue>
              {(selectedPage) => `${String(selectedPage)}/${safeTotalPages}`}
            </SelectValue>
          </SelectTrigger>
          <SelectContent align="end">
            {pageOptionValues.map((pageOption, optionIndex) => {
              const previousOption = pageOptionValues[optionIndex - 1];
              const isGapBefore =
                previousOption !== undefined && pageOption - previousOption > 1;
              return (
                <Fragment key={pageOption}>
                  {isGapBefore ? (
                    <SelectItem value={`page-gap-${pageOption}`} disabled>
                      …
                    </SelectItem>
                  ) : null}
                  <SelectItem value={pageOption}>
                    {String(pageOption)}
                  </SelectItem>
                </Fragment>
              );
            })}
          </SelectContent>
        </Select>
        <span>{pageLabel}</span>
        <Select
          value={pageSize}
          items={pageSizeOptions.map((sizeOption) => ({
            value: sizeOption,
            label: String(sizeOption),
          }))}
          onValueChange={(nextValue) => {
            if (typeof nextValue === "number") {
              onPageSizeChange(nextValue);
            }
          }}
        >
          <SelectTrigger
            size="sm"
            aria-label={pageSizeSelectAriaLabel}
            className="h-8 min-w-[68px] gap-1 px-2 text-[13px]"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="end">
            {pageSizeOptions.map((sizeOption) => (
              <SelectItem key={sizeOption} value={sizeOption}>
                {String(sizeOption)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span>{perPageLabel}</span>
      </div>
    </div>
  );
}
