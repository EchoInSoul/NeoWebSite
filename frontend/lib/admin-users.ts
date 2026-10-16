import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";
import { parseAdminUsers, type AdminUser } from "@/lib/users";

export type AdminUsersRead =
  | { status: "ok"; users: AdminUser[] }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getAdminUsers(): Promise<AdminUsersRead> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  let response: Response;
  try {
    response = await fetch(`${backendUrl}/api/v1/admin/users`, {
      headers: { cookie: `session=${session}` },
      cache: "no-store",
    });
  } catch {
    return { status: "failed" };
  }

  if (response.status === 403) {
    return { status: "forbidden" };
  }

  if (!response.ok) {
    return { status: "failed" };
  }

  const payload = await response.json().catch(() => null);
  const users = parseAdminUsers(payload);

  if (users === null) {
    return { status: "failed" };
  }

  return { status: "ok", users };
}
