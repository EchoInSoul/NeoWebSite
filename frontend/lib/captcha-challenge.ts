import { apiRequest } from "@/lib/api";
import type { CaptchaScope } from "@/lib/captcha-settings";

export type SlideCaptchaChallenge = {
  mode: "slide";
  id: string;
  masterImage: string;
  tileImage: string;
  masterWidth: number;
  masterHeight: number;
  tileWidth: number;
  tileHeight: number;
  tileX: number;
  tileY: number;
  expiresAt: string;
};

export type ClickCaptchaChallenge = {
  mode: "click";
  id: string;
  masterImage: string;
  promptImage: string;
  promptText: string;
  masterWidth: number;
  masterHeight: number;
  expectedClicks: number;
  expiresAt: string;
};

export type CaptchaChallenge = SlideCaptchaChallenge | ClickCaptchaChallenge;

export type CaptchaClick = {
  x: number;
  y: number;
};

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isPositiveNumber(value: unknown): value is number {
  return isFiniteNumber(value) && value > 0;
}

function isPositiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value > 0;
}

function looksLikeSlide(record: Record<string, unknown>): boolean {
  return (
    typeof record.tile_image === "string" &&
    record.tile_image !== "" &&
    isPositiveNumber(record.tile_width) &&
    isPositiveNumber(record.tile_height)
  );
}

function looksLikeClick(record: Record<string, unknown>): boolean {
  return (
    typeof record.prompt_image === "string" &&
    record.prompt_image !== "" &&
    typeof record.prompt_text === "string" &&
    record.prompt_text !== "" &&
    isPositiveInteger(record.expected_clicks)
  );
}

function toSlideChallenge(
  record: Record<string, unknown>,
): SlideCaptchaChallenge {
  if (
    typeof record.id !== "string" ||
    typeof record.master_image !== "string" ||
    typeof record.tile_image !== "string" ||
    typeof record.expires_at !== "string" ||
    !isPositiveNumber(record.master_width) ||
    !isPositiveNumber(record.master_height) ||
    !isPositiveNumber(record.tile_width) ||
    !isPositiveNumber(record.tile_height) ||
    !isFiniteNumber(record.tile_x) ||
    !isFiniteNumber(record.tile_y)
  ) {
    throw new Error("captcha challenge payload malformed");
  }

  return {
    mode: "slide",
    id: record.id,
    masterImage: record.master_image,
    tileImage: record.tile_image,
    masterWidth: record.master_width,
    masterHeight: record.master_height,
    tileWidth: record.tile_width,
    tileHeight: record.tile_height,
    tileX: record.tile_x,
    tileY: record.tile_y,
    expiresAt: record.expires_at,
  };
}

function toClickChallenge(
  record: Record<string, unknown>,
): ClickCaptchaChallenge {
  if (
    typeof record.id !== "string" ||
    typeof record.master_image !== "string" ||
    typeof record.prompt_image !== "string" ||
    typeof record.prompt_text !== "string" ||
    record.prompt_text === "" ||
    typeof record.expires_at !== "string" ||
    !isPositiveNumber(record.master_width) ||
    !isPositiveNumber(record.master_height) ||
    !isPositiveInteger(record.expected_clicks)
  ) {
    throw new Error("captcha challenge payload malformed");
  }

  return {
    mode: "click",
    id: record.id,
    masterImage: record.master_image,
    promptImage: record.prompt_image,
    promptText: record.prompt_text,
    masterWidth: record.master_width,
    masterHeight: record.master_height,
    expectedClicks: record.expected_clicks,
    expiresAt: record.expires_at,
  };
}

function toChallenge(payload: unknown): CaptchaChallenge {
  if (typeof payload !== "object" || payload === null) {
    throw new Error("captcha challenge payload malformed");
  }

  const record = payload as Record<string, unknown>;
  const slide = looksLikeSlide(record);
  const click = looksLikeClick(record);

  if (slide === click) {
    throw new Error("captcha challenge payload malformed");
  }

  return click ? toClickChallenge(record) : toSlideChallenge(record);
}

function toCaptchaToken(payload: unknown): string {
  if (typeof payload !== "object" || payload === null) {
    throw new Error("captcha solution payload malformed");
  }

  const token = (payload as { captcha_token?: unknown }).captcha_token;

  if (typeof token !== "string" || token === "") {
    throw new Error("captcha solution payload malformed");
  }

  return token;
}

export async function requestCaptchaChallenge(
  scope: CaptchaScope,
  signal?: AbortSignal,
): Promise<CaptchaChallenge> {
  const payload = await apiRequest<unknown>("/api/v1/captcha/challenges", {
    method: "POST",
    body: { scope },
    signal,
  });

  return toChallenge(payload);
}

export async function solveSlideCaptchaChallenge(
  id: string,
  x: number,
  y: number,
  signal?: AbortSignal,
): Promise<string> {
  const payload = await apiRequest<unknown>(
    `/api/v1/captcha/challenges/${encodeURIComponent(id)}/solutions`,
    {
      method: "POST",
      body: { x, y },
      signal,
    },
  );

  return toCaptchaToken(payload);
}

export async function solveClickCaptchaChallenge(
  id: string,
  clicks: CaptchaClick[],
  signal?: AbortSignal,
): Promise<string> {
  const payload = await apiRequest<unknown>(
    `/api/v1/captcha/challenges/${encodeURIComponent(id)}/solutions`,
    {
      method: "POST",
      body: { clicks },
      signal,
    },
  );

  return toCaptchaToken(payload);
}
