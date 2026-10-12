import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
} from "@/components/ui/sidebar";

import { ConsoleSidebarFooter } from "./console-sidebar-footer";

export function ConsoleSidebar({ displayName }: { displayName?: string }) {
  return (
    <Sidebar
      id="console-sidebar"
      collapsible="icon"
      className="top-(--console-banner-h) h-[calc(100svh-var(--console-banner-h))] border-r border-border"
    >
      <SidebarContent />
      <SidebarFooter className="border-t border-border">
        <ConsoleSidebarFooter displayName={displayName} />
      </SidebarFooter>
    </Sidebar>
  );
}
