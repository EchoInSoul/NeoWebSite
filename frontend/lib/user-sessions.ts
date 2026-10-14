import { apiRequest } from "@/lib/api";

export type UserSession = {
  id: string;
  current: boolean;
  userAgent: string;
  ip: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
};

type SessionItemPayload = {
  id?: unknown;
  current?: unknown;
  user_agent?: unknown;
  ip?: unknown;
  created_at?: unknown;
  last_seen_at?: unknown;
  expires_at?: unknown;
};

const malformedResponse =
  "session list failed: malformed /users/me/sessions response";

function readText(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function toSession(item: SessionItemPayload): UserSession {
  if (typeof item?.id !== "string" || typeof item.current !== "boolean") {
    throw new Error(malformedResponse);
  }

  return {
    id: item.id,
    current: item.current,
    userAgent: readText(item.user_agent),
    ip: readText(item.ip),
    createdAt: readText(item.created_at),
    lastSeenAt: readText(item.last_seen_at),
    expiresAt: readText(item.expires_at),
  };
}

export function parseUserSessions(payload: unknown): UserSession[] {
  const items = (payload as { items?: unknown } | null)?.items;
  if (!Array.isArray(items)) {
    throw new Error(malformedResponse);
  }

  return items.map((item) => toSession(item as SessionItemPayload));
}

export const userSessionsPath = "/api/v1/users/me/sessions";

export async function fetchUserSessions(): Promise<UserSession[]> {
  return parseUserSessions(await apiRequest(userSessionsPath));
}

export async function revokeUserSession(sessionId: string): Promise<void> {
  await apiRequest(`${userSessionsPath}/${encodeURIComponent(sessionId)}`, {
    method: "DELETE",
  });
}
