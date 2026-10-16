import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
} from "@/components/ui/sidebar";

import { ConsoleSidebarFooter } from "./console-sidebar-footer";
import { ConsoleSidebarNav } from "./console-sidebar-nav";

export function ConsoleSidebar({
  displayName,
  canViewSettings = false,
  canViewRegister = false,
  canViewGroups = false,
  canViewUsers = false,
}: {
  displayName?: string;
  canViewSettings?: boolean;
  canViewRegister?: boolean;
  canViewGroups?: boolean;
  canViewUsers?: boolean;
}) {
  return (
    <Sidebar
      id="console-sidebar"
      collapsible="icon"
      className="top-(--console-banner-h) h-[calc(100svh-var(--console-banner-h))] border-r border-border"
    >
      <SidebarContent>
        <ConsoleSidebarNav
          canViewSettings={canViewSettings}
          canViewRegister={canViewRegister}
          canViewGroups={canViewGroups}
          canViewUsers={canViewUsers}
        />
      </SidebarContent>
      <SidebarFooter className="border-t border-border">
        <ConsoleSidebarFooter displayName={displayName} />
      </SidebarFooter>
    </Sidebar>
  );
}
