import { parseRetryAfter } from "@/lib/rate-limit";
import { reportSessionExpired } from "@/lib/session-expiry";

type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

type RequestOptions = {
  method?: HttpMethod;
  body?: unknown;
  signal?: AbortSignal;
};

type ErrorEnvelope = {
  error?: {
    type?: string;
    code?: string;
    param?: string | null;
  };
};

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly param?: string;
  readonly retryAfter?: number;

  constructor(
    status: number,
    code: string,
    param?: string,
    retryAfter?: number,
  ) {
    super(code);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.param = param;
    this.retryAfter = retryAfter;
  }
}

export function looksLikeServiceFailure(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 0 || error.status >= 500)
  );
}

async function readPayload(response: Response) {
  const text = await response.text();
  if (text.length === 0) {
    return null;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return null;
  }
}

export async function apiRequest<T = unknown>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = "GET", body, signal } = options;

  let response: Response;
  try {
    response = await fetch(path, {
      method,
      credentials: "same-origin",
      headers:
        body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    });
  } catch {
    throw new ApiError(0, "network_error");
  }

  const payload = await readPayload(response);

  if (!response.ok) {
    const error = (payload as ErrorEnvelope | null)?.error;
    if (response.status === 401 && error?.code === "unauthorized") {
      reportSessionExpired();
    }
    throw new ApiError(
      response.status,
      error?.code ?? "invalid_response",
      error?.param ?? undefined,
      parseRetryAfter(response.headers.get("Retry-After")),
    );
  }

  return payload as T;
}

let healthProbe: Promise<boolean> | null = null;

export function isServiceUp(): Promise<boolean> {
  if (healthProbe === null) {
    healthProbe = apiRequest("/api/v1/healthy")
      .then(
        () => true,
        () => false,
      )
      .finally(() => {
        healthProbe = null;
      });
  }

  return healthProbe;
}
