import type { ReactNode } from "react";
import { cn } from "cn";

import { Card, CardContent } from "@/components/ui/card";

export function ConsoleSettingsGroup({
  title,
  description,
  children,
  className,
}: {
  title: string;
  description?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("flex min-w-0 flex-col gap-2", className)}>
      <div className="flex min-w-0 flex-col gap-1">
        <h3 className="text-sm font-semibold">{title}</h3>
        {description === undefined ? null : (
          <p className="text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      <Card size="none">
        <CardContent className="divide-y divide-border">{children}</CardContent>
      </Card>
    </section>
  );
}
