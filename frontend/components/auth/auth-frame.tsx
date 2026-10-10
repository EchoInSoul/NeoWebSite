import type { ReactNode } from "react";

import { HaoBlogWordmark } from "@/components/brand/haoblog-wordmark";

export function AuthFrame({
  children,
  footer,
}: {
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <main className="flex min-h-svh w-full flex-col items-center justify-center bg-background px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="mb-4 flex justify-center">
          <HaoBlogWordmark className="h-12 w-auto" />
        </div>
        <div className="rounded-lg border border-border bg-card p-6">
          {children}
        </div>
        {footer}
      </div>
    </main>
  );
}
