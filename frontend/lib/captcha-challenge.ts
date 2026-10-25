import { apiRequest } from "@/lib/api";
import type { CaptchaScope } from "@/lib/captcha-settings";

export type CaptchaChallenge = {
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

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isPositiveNumber(value: unknown): value is number {
  return isFiniteNumber(value) && value > 0;
}

function toChallenge(payload: unknown): CaptchaChallenge {
  if (typeof payload !== "object" || payload === null) {
    throw new Error("captcha challenge payload malformed");
  }

  const record = payload as Record<string, unknown>;

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

export async function solveCaptchaChallenge(
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

  if (typeof payload !== "object" || payload === null) {
    throw new Error("captcha solution payload malformed");
  }

  const token = (payload as { captcha_token?: unknown }).captcha_token;

  if (typeof token !== "string" || token === "") {
    throw new Error("captcha solution payload malformed");
  }

  return token;
}
