const QUICK_SCALE_KEYFRAMES: Keyframe[] = [
  { transform: "scale(1)" },
  { transform: "scale(0.985)", offset: 0.32 },
  { transform: "scale(1.006)", offset: 0.68 },
  { transform: "scale(1)" },
];

export function nudgeDialogNode(dialogNode: HTMLElement | null): void {
  if (dialogNode === null) {
    return;
  }
  if (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  ) {
    return;
  }
  dialogNode.animate(QUICK_SCALE_KEYFRAMES, {
    duration: 140,
    easing: "ease",
  });
}
