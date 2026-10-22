import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { ConsoleServiceToast } from "@/components/console/console-service-toast";
import { ConsoleSidebar } from "@/components/console/console-sidebar";
import { ConsoleSigninNotice } from "@/components/console/console-signin-notice";
import { ConsoleTopBanner } from "@/components/console/console-top-banner";
import { SessionExpiryDialog } from "@/components/console/session-expiry-dialog";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { getAuthenticatedUser, type SessionUser } from "@/lib/auth-session";

export const instant = false;

function ConsoleShell({
  displayName,
  canViewSettings = false,
  canViewRegister = false,
  canViewGroups = false,
  canViewUsers = false,
  children,
}: {
  displayName?: string;
  canViewSettings?: boolean;
  canViewRegister?: boolean;
  canViewGroups?: boolean;
  canViewUsers?: boolean;
  children: ReactNode;
}) {
  return (
    <SidebarProvider className="h-svh min-h-0 flex-col overflow-hidden bg-background">
      <ConsoleTopBanner />
      <div className="flex min-h-0 flex-1">
        <ConsoleSidebar
          displayName={displayName}
          canViewSettings={canViewSettings}
          canViewRegister={canViewRegister}
          canViewGroups={canViewGroups}
          canViewUsers={canViewUsers}
        />
        <SidebarInset className="overflow-y-auto">{children}</SidebarInset>
      </div>
      <SessionExpiryDialog />
      <ConsoleSigninNotice />
    </SidebarProvider>
  );
}

export default async function ConsoleLayout({
  children,
}: LayoutProps<"/console">) {
  let user: SessionUser | null;
  try {
    user = await getAuthenticatedUser();
  } catch {
    return (
      <>
        <ConsoleShell>{children}</ConsoleShell>
        <ConsoleServiceToast />
      </>
    );
  }

  if (!user) {
    redirect("/signin");
  }

  return (
    <ConsoleShell
      displayName={user.nickname}
      canViewSettings={user.permissions.includes("system.settings.view")}
      canViewRegister={user.permissions.includes("system.register.view")}
      canViewGroups={user.permissions.includes("system.groups.view")}
      canViewUsers={user.permissions.includes("system.users.view")}
    >
      {children}
    </ConsoleShell>
  );
}
