"use client";

import { useState, type ReactNode } from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { cn } from "cn";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item";

export type ConsoleSettingsRowProperties = {
  label: string;
  description?: ReactNode;
  icon?: ReactNode;
  control?: ReactNode;
  href?: string;
  expandedContent?: ReactNode;
  defaultExpanded?: boolean;
  className?: string;
};

const rowOverlayClassName =
  "absolute inset-0 z-0 cursor-pointer transition-colors outline-none hover:bg-muted focus-visible:border focus-visible:border-ring";

const rowLayerClassName = "pointer-events-none relative z-10";

const rowExpandedClassName = "divide-y divide-border border-t border-border";

export function ConsoleSettingsRow({
  label,
  description,
  icon,
  control,
  href,
  expandedContent,
  defaultExpanded = false,
  className,
}: ConsoleSettingsRowProperties) {
  const [isExpanded, setIsExpanded] = useState<boolean>(defaultExpanded);
  const isLinkRow = href !== undefined;
  const isExpandableRow = expandedContent !== undefined;
  const isInteractiveRow = isLinkRow || isExpandableRow;

  const rowContentNode = (
    <>
      {icon === undefined ? null : (
        <ItemMedia
          variant="icon"
          className={isInteractiveRow ? rowLayerClassName : undefined}
        >
          {icon}
        </ItemMedia>
      )}
      <ItemContent className={isInteractiveRow ? rowLayerClassName : undefined}>
        <ItemTitle>{label}</ItemTitle>
        {description === undefined ? null : (
          <ItemDescription>{description}</ItemDescription>
        )}
      </ItemContent>
      {control === undefined ? null : <ItemActions>{control}</ItemActions>}
      {isInteractiveRow ? (
        <span className={cn(rowLayerClassName, "self-center")}>
          <ChevronRight
            aria-hidden="true"
            className={cn(
              "size-4 shrink-0 text-muted-foreground transition-transform duration-200",
              isExpandableRow && isExpanded ? "rotate-90" : null,
            )}
          />
        </span>
      ) : null}
    </>
  );

  if (isExpandableRow) {
    return (
      <Collapsible
        className={className}
        open={isExpanded}
        onOpenChange={setIsExpanded}
      >
        <Item variant="flush" size="lg">
          <CollapsibleTrigger className={rowOverlayClassName}>
            <span className="sr-only">{label}</span>
          </CollapsibleTrigger>
          {rowContentNode}
        </Item>
        <CollapsibleContent className={rowExpandedClassName}>
          {expandedContent}
        </CollapsibleContent>
      </Collapsible>
    );
  }

  return (
    <Item className={className} variant="flush" size="lg">
      {isLinkRow ? (
        <Link className={rowOverlayClassName} href={href}>
          <span className="sr-only">{label}</span>
        </Link>
      ) : null}
      {rowContentNode}
    </Item>
  );
}
