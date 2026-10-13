export const SIDEBAR_PANEL_TRANSITION =
  "transition-[transform,translate,visibility,width] duration-[360ms] ease-[cubic-bezier(0.32,0.72,0,1)] motion-reduce:transition-none";

export const SIDEBAR_LABEL_TRANSITION =
  "truncate transition-[max-width,opacity] motion-reduce:transition-none";

export function sidebarLabelFade(isCollapsed: boolean) {
  return isCollapsed
    ? "pointer-events-none max-w-0 opacity-0 delay-0 duration-150 ease-[cubic-bezier(0.4,0,1,1)]"
    : "max-w-52 opacity-100 delay-75 duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]";
}
