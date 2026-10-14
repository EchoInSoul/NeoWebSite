import { useCallback, useEffect, useRef, useState } from "react";

const feedbackMs = 1600;

export function useCopyToClipboard() {
  const [isCopied, setIsCopied] = useState(false);
  const resetTimer = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (resetTimer.current !== null) {
        window.clearTimeout(resetTimer.current);
      }
    };
  }, []);

  const copy = useCallback(async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      return;
    }

    setIsCopied(true);

    if (resetTimer.current !== null) {
      window.clearTimeout(resetTimer.current);
    }
    resetTimer.current = window.setTimeout(
      () => setIsCopied(false),
      feedbackMs,
    );
  }, []);

  return { isCopied, copy };
}
