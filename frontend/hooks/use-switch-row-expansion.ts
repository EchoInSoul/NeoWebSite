import { useState } from "react";

export function useSwitchRowExpansion(initiallyEnabled: boolean) {
  const [expanded, setExpanded] = useState(initiallyEnabled);

  function expandOnEnable(nextEnabled: boolean) {
    if (nextEnabled) {
      setExpanded(true);
    }
  }

  return { expanded, setExpanded, expandOnEnable };
}
