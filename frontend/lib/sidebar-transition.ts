export const SIDEBAR_PANEL_TRANSITION =
  "transition-[transform,translate,visibility,width] duration-[360ms] ease-[cubic-bezier(0.32,0.72,0,1)] motion-reduce:transition-none";

export const SIDEBAR_LABEL_TRANSITION =
  "truncate transition-[max-width,opacity] motion-reduce:transition-none";

export function sidebarLabelFade(isCollapsed: boolean) {
  return isCollapsed
    ? "pointer-events-none max-w-0 opacity-0 delay-0 duration-150 ease-[cubic-bezier(0.4,0,1,1)]"
    : "max-w-52 opacity-100 delay-75 duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]";
}

export const SIDEBAR_SLIDE_TRANSITION =
  "transition-[translate] duration-[360ms] ease-[cubic-bezier(0.32,0.72,0,1)] motion-reduce:transition-none";

export const SIDEBAR_SLIDE_DURATION_MS = 360;

export type SidebarSlideOffset = "none" | "left" | "right";

export function sidebarSlideOffsetClass(
  offset: SidebarSlideOffset,
  isCollapsed: boolean,
) {
  if (offset === "none") {
    return "translate-x-0";
  }
  if (isCollapsed) {
    return offset === "left" ? "-translate-x-[20px]" : "translate-x-[20px]";
  }
  return offset === "left" ? "-translate-x-[99px]" : "translate-x-[99px]";
}
