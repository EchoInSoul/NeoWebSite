"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowRight, Check, RefreshCw, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";
import { cn } from "cn";

import {
  requestCaptchaChallenge,
  solveCaptchaChallenge,
  type CaptchaChallenge,
} from "@/lib/captcha-challenge";
import type { CaptchaScope } from "@/lib/captcha-settings";
import { nudgeDialogNode } from "@/lib/dialog-nudge";
import { ApiError } from "@/lib/api";

const CAPTCHA_FAIL_RETRY_MS = 800;
const DEFAULT_CAPTCHA_CANVAS = { width: 300, height: 220 };

type Phase =
  "loading" | "ready" | "dragging" | "verifying" | "success" | "failed";

export function BuiltinCaptcha({
  scope,
  initialChallenge,
  onToken,
}: {
  scope: CaptchaScope;
  initialChallenge: CaptchaChallenge | null;
  onToken: (token: string) => void;
}) {
  const t = useTranslations("auth");
  const [challenge, setChallenge] = useState<CaptchaChallenge | null>(
    initialChallenge,
  );
  const [phase, setPhase] = useState<Phase>(
    initialChallenge === null ? "loading" : "ready",
  );
  const [slidePx, setSlidePx] = useState(0);
  const [renderedWidth, setRenderedWidth] = useState(0);
  const [elapsedSeconds, setElapsedSeconds] = useState<string | null>(null);
  const hostRef = useRef<HTMLDivElement | null>(null);
  const challengeControllerRef = useRef<AbortController | null>(null);
  const solveControllerRef = useRef<AbortController | null>(null);
  const retryTimerRef = useRef<number | null>(null);
  const originRef = useRef({ x: 0, y: 0 });
  const dragStartRef = useRef(0);
  const slidePxRef = useRef(0);
  const requestedRef = useRef(initialChallenge !== null);
  const onTokenRef = useRef(onToken);

  useEffect(() => {
    onTokenRef.current = onToken;
  }, [onToken]);

  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current !== null) {
      window.clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    return () => {
      clearRetryTimer();
      challengeControllerRef.current?.abort();
      solveControllerRef.current?.abort();
    };
  }, [clearRetryTimer]);

  useEffect(() => {
    const element = hostRef.current;
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

  const requestChallenge = useCallback(async () => {
    challengeControllerRef.current?.abort();
    const controller = new AbortController();
    challengeControllerRef.current = controller;

    try {
      const next = await requestCaptchaChallenge(scope, controller.signal);
      if (controller.signal.aborted) {
        return;
      }
      slidePxRef.current = 0;
      setSlidePx(0);
      setElapsedSeconds(null);
      setChallenge(next);
      setPhase("ready");
    } catch (error) {
      if (controller.signal.aborted) {
        return;
      }
      if (error instanceof ApiError && error.status === 429) {
        toast.error(t("captchaRateLimited"));
      }
      setChallenge(null);
      setPhase("failed");
    } finally {
      if (challengeControllerRef.current === controller) {
        challengeControllerRef.current = null;
      }
    }
  }, [scope, t]);

  useEffect(() => {
    if (requestedRef.current) {
      return;
    }
    requestedRef.current = true;
    void requestChallenge();
  }, [requestChallenge]);

  const scale =
    challenge === null || renderedWidth === 0
      ? 1
      : renderedWidth / challenge.masterWidth;
  const pieceWidth = challenge === null ? 0 : challenge.tileWidth * scale;
  const pieceHeight = challenge === null ? 0 : challenge.tileHeight * scale;
  const dragMax =
    challenge === null || renderedWidth === 0
      ? 0
      : Math.max(
          0,
          (challenge.masterWidth - challenge.tileWidth - challenge.tileX) *
            scale,
        );
  const isLocked =
    phase === "verifying" || phase === "success" || phase === "failed";

  function beginDrag(event: React.PointerEvent<HTMLDivElement>) {
    if (phase !== "ready" || challenge === null || renderedWidth === 0) {
      return;
    }
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    originRef.current = { x: event.clientX, y: event.clientY };
    dragStartRef.current = performance.now();
    setPhase("dragging");
  }

  function moveDrag(event: React.PointerEvent<HTMLDivElement>) {
    if (phase !== "dragging") {
      return;
    }
    event.preventDefault();
    const next = Math.max(
      0,
      Math.min(dragMax, event.clientX - originRef.current.x),
    );
    slidePxRef.current = next;
    setSlidePx(next);
  }

  async function submitSlide() {
    if (challenge === null || renderedWidth === 0) {
      return;
    }

    const currentScale = renderedWidth / challenge.masterWidth;
    const x = Math.round(challenge.tileX + slidePxRef.current / currentScale);
    const y = challenge.tileY;
    const controller = new AbortController();
    solveControllerRef.current = controller;
    setPhase("verifying");

    try {
      const token = await solveCaptchaChallenge(
        challenge.id,
        x,
        y,
        controller.signal,
      );
      if (controller.signal.aborted) {
        return;
      }
      const elapsed = performance.now() - dragStartRef.current;
      const seconds = Math.max(0.1, Math.round(elapsed / 100) / 10).toFixed(1);
      setElapsedSeconds(seconds);
      onTokenRef.current(token);
      setPhase("success");
    } catch {
      if (controller.signal.aborted) {
        return;
      }
      const dialogNode =
        hostRef.current?.closest<HTMLElement>('[role="dialog"]') ?? null;
      nudgeDialogNode(dialogNode);
      setPhase("failed");
      clearRetryTimer();
      retryTimerRef.current = window.setTimeout(() => {
        retryTimerRef.current = null;
        void requestChallenge();
      }, CAPTCHA_FAIL_RETRY_MS);
    } finally {
      if (solveControllerRef.current === controller) {
        solveControllerRef.current = null;
      }
    }
  }

  function endDrag(event: React.PointerEvent<HTMLDivElement>) {
    if (phase !== "dragging") {
      return;
    }
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    void submitSlide();
  }

  return (
    <div ref={hostRef} className="flex w-full flex-col gap-3 select-none">
      {challenge === null ? (
        phase === "failed" ? (
          <div
            role="alert"
            style={{
              aspectRatio: `${DEFAULT_CAPTCHA_CANVAS.width} / ${DEFAULT_CAPTCHA_CANVAS.height}`,
            }}
            className="flex w-full flex-col items-center justify-center gap-2 rounded-lg border border-red-300 bg-red-50 text-sm text-red-600 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
          >
            <span>{t("captchaLoadFailed")}</span>
            <button
              type="button"
              onClick={() => {
                void requestChallenge();
              }}
              className="inline-flex items-center gap-1 rounded-md border border-red-300 px-2 py-1 text-xs hover:bg-red-100 dark:border-red-900/60 dark:hover:bg-red-900/40"
            >
              <RefreshCw className="h-3 w-3" aria-hidden="true" />
              {t("captchaReload")}
            </button>
          </div>
        ) : null
      ) : (
        <>
          <div
            className="relative w-full overflow-hidden bg-muted/20"
            style={{
              aspectRatio: `${challenge.masterWidth} / ${challenge.masterHeight}`,
            }}
          >
            <img
              src={challenge.masterImage}
              alt={t("captchaImageAlt")}
              className="absolute inset-0 h-full w-full object-cover select-none"
              draggable={false}
            />
            <img
              src={challenge.tileImage}
              alt=""
              className={cn(
                "absolute object-cover drop-shadow-md transition-[filter] will-change-transform select-none",
                phase === "success" && "ring-2 ring-emerald-400",
                phase === "failed" && "ring-2 ring-red-400",
              )}
              style={{
                width: pieceWidth,
                height: pieceHeight,
                top: challenge.tileY * scale,
                left: 0,
                transform: `translateX(${challenge.tileX * scale + slidePx}px)`,
              }}
              draggable={false}
            />
            {phase === "success" ? (
              <>
                <div className="absolute inset-0 bg-emerald-500/30" />
                <div className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-1 bg-emerald-500/85 px-2 py-1 text-xs font-medium text-white">
                  <Check className="h-3 w-3" aria-hidden="true" />
                  {elapsedSeconds === null
                    ? null
                    : t("captchaSuccess", { seconds: elapsedSeconds })}
                </div>
              </>
            ) : null}
            {phase === "failed" ? (
              <div className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-1 bg-red-500/85 px-2 py-1 text-xs font-medium text-white">
                <X className="h-3 w-3" aria-hidden="true" />
                {t("captchaError")}
              </div>
            ) : null}
          </div>
          <div
            role="slider"
            aria-label={t("captchaSliderHint")}
            aria-valuemin={0}
            aria-valuemax={Math.max(1, Math.round(dragMax))}
            aria-valuenow={Math.round(slidePx)}
            aria-disabled={isLocked}
            tabIndex={0}
            data-state={phase}
            onPointerDown={isLocked ? undefined : beginDrag}
            onPointerMove={isLocked ? undefined : moveDrag}
            onPointerUp={isLocked ? undefined : endDrag}
            onPointerCancel={isLocked ? undefined : endDrag}
            className={cn(
              "relative h-10 w-full touch-none rounded-sm border border-border bg-muted/30 outline-none select-none",
              isLocked
                ? "cursor-default opacity-90"
                : "cursor-grab active:cursor-grabbing",
            )}
          >
            <div
              className={cn(
                "pointer-events-none absolute inset-y-0 left-0 rounded-l-sm transition-colors",
                phase === "success"
                  ? "bg-emerald-200/80 dark:bg-emerald-900/50"
                  : "bg-primary/10",
              )}
              style={{ width: slidePx + Math.min(pieceWidth, 40) }}
            />
            <div
              className={cn(
                "absolute top-0 left-0 flex h-10 w-10 items-center justify-center rounded-sm border border-border bg-card text-muted-foreground shadow-sm",
                phase === "success" &&
                  "border-emerald-400 bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-200",
                phase === "failed" &&
                  "border-red-400 bg-red-100 text-red-600 dark:bg-red-900/40 dark:text-red-200",
              )}
              style={{ transform: `translateX(${slidePx}px)` }}
            >
              {phase === "success" ? (
                <Check className="h-4 w-4" aria-hidden="true" />
              ) : phase === "failed" ? (
                <X className="h-4 w-4" aria-hidden="true" />
              ) : (
                <ArrowRight className="h-4 w-4" aria-hidden="true" />
              )}
            </div>
            {slidePx === 0 && !isLocked ? (
              <div className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">
                {t("captchaSliderHint")}
              </div>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
}
