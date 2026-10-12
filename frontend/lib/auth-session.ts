import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";

export type SessionUser = {
  email: string;
  nickname: string;
};

type MeResponse = {
  email?: unknown;
  nickname?: unknown;
};

export async function getAuthenticatedUser(): Promise<SessionUser | null> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return null;
  }

  let response: Response;
  try {
    response = await fetch(`${backendUrl}/api/v1/auth/me`, {
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

  if (typeof me?.email !== "string" || typeof me?.nickname !== "string") {
    throw new Error("session lookup failed: malformed /auth/me response");
  }

  return { email: me.email, nickname: me.nickname };
}
