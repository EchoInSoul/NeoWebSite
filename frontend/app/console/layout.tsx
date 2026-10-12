import { redirect } from "next/navigation";

import { ConsoleSidebar } from "@/components/console/console-sidebar";
import { ConsoleTopBanner } from "@/components/console/console-top-banner";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { getAuthenticatedUser } from "@/lib/auth-session";

export default async function ConsoleLayout({
  children,
}: LayoutProps<"/console">) {
  const user = await getAuthenticatedUser();

  if (!user) {
    redirect("/signin");
  }

  return (
    <SidebarProvider className="h-svh min-h-0 flex-col overflow-hidden bg-background">
      <ConsoleTopBanner />
      <div className="flex min-h-0 flex-1">
        <ConsoleSidebar displayName={user.nickname} />
        <SidebarInset className="overflow-y-auto">{children}</SidebarInset>
      </div>
    </SidebarProvider>
  );
}
