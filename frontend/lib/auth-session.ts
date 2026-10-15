import { cache } from "react";
import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";

export type SessionGroup = {
  id: string;
  systemKey: string | null;
  name: string;
};

export type SessionUser = {
  email: string;
  nickname: string;
  bio: string | null;
  group: SessionGroup;
  twoFactorEnabled: boolean;
};

type MeResponse = {
  email?: unknown;
  nickname?: unknown;
  bio?: unknown;
  group?: unknown;
  two_factor_enabled?: unknown;
};

const malformedResponse = "session lookup failed: malformed /users/me response";

function parseGroup(value: unknown): SessionGroup {
  const group = value as
    { id?: unknown; system_key?: unknown; name?: unknown } | null | undefined;

  if (typeof group?.id !== "string" || typeof group.name !== "string") {
    throw new Error(malformedResponse);
  }

  const systemKey = group.system_key;

  if (systemKey !== null && typeof systemKey !== "string") {
    throw new Error(malformedResponse);
  }

  return { id: group.id, systemKey, name: group.name };
}

export const getAuthenticatedUser = cache(
  async (): Promise<SessionUser | null> => {
    const session = (await cookies()).get("session")?.value;

    if (!session) {
      return null;
    }

    let response: Response;
    try {
      response = await fetch(`${backendUrl}/api/v1/users/me`, {
        headers: { cookie: `session=${session}` },
      });
    } catch {
      throw new Error("session lookup failed: backend unreachable");
    }

    if (response.status === 401) {
      return null;
    }

    if (!response.ok) {
      throw new Error(`session lookup failed: status ${response.status}`);
    }

    const me = (await response.json().catch(() => null)) as MeResponse | null;

    if (
      typeof me?.email !== "string" ||
      typeof me.nickname !== "string" ||
      typeof me.two_factor_enabled !== "boolean"
    ) {
      throw new Error(malformedResponse);
    }

    const bio = me.bio;

    if (bio !== null && bio !== undefined && typeof bio !== "string") {
      throw new Error(malformedResponse);
    }

    return {
      email: me.email,
      nickname: me.nickname,
      bio: bio ?? null,
      group: parseGroup(me.group),
      twoFactorEnabled: me.two_factor_enabled,
    };
  },
);
