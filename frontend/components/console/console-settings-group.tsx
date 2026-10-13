import type { ReactNode } from "react";
import { cn } from "cn";

import { Card, CardContent } from "@/components/ui/card";

export function ConsoleSettingsGroup({
  title,
  children,
  className,
}: {
  title: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("flex min-w-0 flex-col gap-2", className)}>
      <h3 className="text-sm font-semibold">{title}</h3>
      <Card size="none">
        <CardContent className="divide-y divide-border">{children}</CardContent>
      </Card>
    </section>
  );
}
