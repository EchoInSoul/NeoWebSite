"use client";

import { useEffect, useRef, useState } from "react";
import { Check, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { cn } from "cn";

import {
  isCaptchaRejected,
  solveClickCaptchaChallenge,
  type CaptchaClick,
  type ClickCaptchaChallenge,
} from "@/lib/captcha-challenge";

type ClickPhase = "ready" | "verifying" | "success" | "failed";

export function ClickCaptcha({
  challenge,
  onToken,
  onRejected,
  onServiceError,
}: {
  challenge: ClickCaptchaChallenge;
  onToken: (token: string) => void;
  onRejected: () => void;
  onServiceError: (error: unknown) => void;
}) {
  const t = useTranslations("auth");
  const [phase, setPhase] = useState<ClickPhase>("ready");
  const [clicks, setClicks] = useState<CaptchaClick[]>([]);
  const [renderedWidth, setRenderedWidth] = useState(0);
  const [elapsedSeconds, setElapsedSeconds] = useState<string | null>(null);
  const imageRef = useRef<HTMLDivElement | null>(null);
  const solveControllerRef = useRef<AbortController | null>(null);
  const clicksRef = useRef<CaptchaClick[]>([]);
  const firstClickAtRef = useRef(0);
  const lockedRef = useRef(false);
  const onTokenRef = useRef(onToken);
  const onRejectedRef = useRef(onRejected);
  const onServiceErrorRef = useRef(onServiceError);

  useEffect(() => {
    onTokenRef.current = onToken;
  }, [onToken]);

  useEffect(() => {
    onRejectedRef.current = onRejected;
  }, [onRejected]);

  useEffect(() => {
    onServiceErrorRef.current = onServiceError;
  }, [onServiceError]);

  useEffect(() => {
    return () => {
      solveControllerRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    const element = imageRef.current;
    if (element === null) {
      return;
    }
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setRenderedWidth(entry.contentRect.width);
      }
    });
    observer.observe(element);
    setRenderedWidth(element.getBoundingClientRect().width);
    return () => observer.disconnect();
  }, []);

  const scale = renderedWidth === 0 ? 1 : renderedWidth / challenge.masterWidth;
  const promptChars = challenge.promptText.split(" ").filter(Boolean).join(",");

  async function submitClicks(points: CaptchaClick[]) {
    const controller = new AbortController();
    solveControllerRef.current = controller;
    setPhase("verifying");

    try {
      const token = await solveClickCaptchaChallenge(
        challenge.id,
        points,
        controller.signal,
      );
      if (controller.signal.aborted) {
        return;
      }
      const elapsed = performance.now() - firstClickAtRef.current;
      const seconds = Math.max(0.1, Math.round(elapsed / 100) / 10).toFixed(1);
      setElapsedSeconds(seconds);
      onTokenRef.current(token);
      setPhase("success");
    } catch (error) {
      if (controller.signal.aborted) {
        return;
      }
      if (isCaptchaRejected(error)) {
        onRejectedRef.current();
        setPhase("failed");
        return;
      }
      onServiceErrorRef.current(error);
    } finally {
      if (solveControllerRef.current === controller) {
        solveControllerRef.current = null;
      }
    }
  }

  function handlePointerDown(event: React.PointerEvent<HTMLDivElement>) {
    if (!event.isPrimary || event.button !== 0) {
      return;
    }
    if (phase !== "ready" || renderedWidth === 0 || lockedRef.current) {
      return;
    }
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width === 0) {
      return;
    }
    event.preventDefault();
    const factor = challenge.masterWidth / rect.width;
    const point = {
      x: Math.round((event.clientX - rect.left) * factor),
      y: Math.round((event.clientY - rect.top) * factor),
    };
    const next = [...clicksRef.current, point];
    clicksRef.current = next;
    if (next.length === 1) {
      firstClickAtRef.current = performance.now();
    }
    setClicks(next);
    if (next.length === challenge.expectedClicks) {
      lockedRef.current = true;
      void submitClicks(next);
    }
  }

  return (
    <>
      <div
        ref={imageRef}
        className="relative w-full cursor-default touch-none overflow-hidden rounded-sm border border-border bg-muted/20 select-none"
        style={{
          aspectRatio: `${challenge.masterWidth} / ${challenge.masterHeight}`,
        }}
        onPointerDown={handlePointerDown}
      >
        <img
          src={challenge.masterImage}
          alt={t("captchaClickImageAlt")}
          className="absolute inset-0 h-full w-full object-cover select-none"
          draggable={false}
        />
        {clicks.map((click, index) => (
          <span
            key={`${click.x}-${click.y}-${index}`}
            className={cn(
              "pointer-events-none absolute flex h-5 w-5 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border text-[10px] font-medium text-white shadow-sm",
              phase === "failed"
                ? "border-red-200/80 bg-red-500/85"
                : "border-white/80 bg-sky-500/85",
            )}
            style={{ left: click.x * scale, top: click.y * scale }}
          >
            {index + 1}
          </span>
        ))}
        {phase === "success" ? (
          <>
            <div className="absolute inset-0 bg-emerald-500/30" />
            <div className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-1 bg-emerald-500/85 px-[7px] py-[3px] text-[10px] font-medium text-white">
              <Check className="h-2.5 w-2.5" aria-hidden="true" />
              {elapsedSeconds === null
                ? null
                : t("captchaSuccess", { seconds: elapsedSeconds })}
            </div>
          </>
        ) : null}
        {phase === "failed" ? (
          <div className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-1 bg-red-500/85 px-[7px] py-[3px] text-[10px] font-medium text-white">
            <X className="h-2.5 w-2.5" aria-hidden="true" />
            {t("captchaError")}
          </div>
        ) : null}
      </div>
      <div className="mt-[25px] flex h-12 w-full items-center justify-center rounded-sm border border-border px-[10px] text-center">
        <span className="text-[13px]">
          {t("captchaClickPrompt", { chars: promptChars })}
        </span>
      </div>
    </>
  );
}
