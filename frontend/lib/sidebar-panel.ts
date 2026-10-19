export const SIDEBAR_PANELS = ["main", "settings", "uc"] as const;

export type SidebarPanel = (typeof SIDEBAR_PANELS)[number];

export type SidebarSubPanel = Exclude<SidebarPanel, "main">;

const SIDEBAR_PANEL_EVENT = "console:sidebar-panel";

export function isSidebarPanel(value: unknown): value is SidebarPanel {
  return SIDEBAR_PANELS.includes(value as SidebarPanel);
}

export function sidebarPanelOrder(panel: SidebarPanel) {
  return SIDEBAR_PANELS.indexOf(panel);
}

export function requestSidebarPanel(panel: SidebarPanel) {
  window.dispatchEvent(
    new CustomEvent<SidebarPanel>(SIDEBAR_PANEL_EVENT, { detail: panel }),
  );
}

export function subscribeSidebarPanel(listener: (panel: SidebarPanel) => void) {
  function handlePanelRequest(event: Event) {
    const panel = (event as CustomEvent<unknown>).detail;

    if (isSidebarPanel(panel)) {
      listener(panel);
    }
  }

  window.addEventListener(SIDEBAR_PANEL_EVENT, handlePanelRequest);

  return () => {
    window.removeEventListener(SIDEBAR_PANEL_EVENT, handlePanelRequest);
  };
}
