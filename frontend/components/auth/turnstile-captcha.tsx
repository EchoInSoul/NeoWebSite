"use client";

import { useEffect, useRef } from "react";
import { useLocale } from "next-intl";

type TurnstileWidgetOptions = {
  sitekey: string;
  theme: "light" | "dark";
  language: string;
  size: "flexible";
  callback: (token: string) => void;
  "expired-callback": () => void;
  "error-callback": () => void;
};

type TurnstileApi = {
  render: (container: HTMLElement, options: TurnstileWidgetOptions) => string;
  remove: (widgetId: string) => void;
};

declare global {
  interface Window {
    turnstile?: TurnstileApi;
  }
}

const SCRIPT_SOURCE =
  "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
const SCRIPT_TIMEOUT_MS = 10_000;

let turnstileLoader: Promise<TurnstileApi> | null = null;

function loadTurnstileApi(): Promise<TurnstileApi> {
  if (window.turnstile) {
    return Promise.resolve(window.turnstile);
  }
  if (turnstileLoader !== null) {
    return turnstileLoader;
  }

  const loader = new Promise<TurnstileApi>((resolve, reject) => {
    const script = document.createElement("script");
    const timer = window.setTimeout(() => {
      script.remove();
      reject(new Error("turnstile script timed out"));
    }, SCRIPT_TIMEOUT_MS);

    script.src = SCRIPT_SOURCE;
    script.async = true;
    script.addEventListener("load", () => {
      window.clearTimeout(timer);
      if (window.turnstile) {
        resolve(window.turnstile);
        return;
      }
      script.remove();
      reject(new Error("turnstile api missing after load"));
    });
    script.addEventListener("error", () => {
      window.clearTimeout(timer);
      script.remove();
      reject(new Error("turnstile script failed to load"));
    });
    document.head.appendChild(script);
  });

  turnstileLoader = loader;
  loader.catch(() => {
    turnstileLoader = null;
  });

  return loader;
}

export function TurnstileCaptcha({
  siteKey,
  onToken,
  onUnavailable,
}: {
  siteKey: string;
  onToken: (token: string) => void;
  onUnavailable: () => void;
}) {
  const locale = useLocale();
  const containerRef = useRef<HTMLDivElement>(null);
  const onTokenRef = useRef(onToken);
  const onUnavailableRef = useRef(onUnavailable);

  useEffect(() => {
    onTokenRef.current = onToken;
    onUnavailableRef.current = onUnavailable;
  }, [onToken, onUnavailable]);

  useEffect(() => {
    let disposed = false;
    let widgetId: string | null = null;

    async function mountWidget() {
      try {
        const api = await loadTurnstileApi();
        if (disposed || containerRef.current === null) {
          return;
        }
        widgetId = api.render(containerRef.current, {
          sitekey: siteKey,
          theme: document.documentElement.classList.contains("dark")
            ? "dark"
            : "light",
          language: locale === "zh-CN" ? "zh-cn" : "en",
          size: "flexible",
          callback: (token) => onTokenRef.current(token),
          "expired-callback": () => onTokenRef.current(""),
          "error-callback": () => onUnavailableRef.current(),
        });
      } catch {
        onUnavailableRef.current();
      }
    }

    void mountWidget();

    return () => {
      disposed = true;
      if (widgetId !== null && window.turnstile) {
        window.turnstile.remove(widgetId);
      }
    };
  }, [siteKey, locale]);

  return <div ref={containerRef} className="flex min-h-16 justify-center" />;
}
