import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";
import {
  parseUserSessions,
  userSessionsPath,
  type UserSession,
} from "@/lib/user-sessions";

export async function getCurrentUserSessions(): Promise<UserSession[] | null> {
  const sessionToken = (await cookies()).get("session")?.value;

  if (sessionToken === undefined) {
    return null;
  }

  try {
    const response = await fetch(`${backendUrl}${userSessionsPath}`, {
      cache: "no-store",
      headers: { cookie: `session=${sessionToken}` },
    });

    if (!response.ok) {
      return null;
    }

    return parseUserSessions(await response.json());
  } catch {
    return null;
  }
}
