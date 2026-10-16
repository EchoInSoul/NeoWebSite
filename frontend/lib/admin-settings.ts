import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";
import { parseAdminGroups } from "@/lib/groups";
import type { AdminGroup } from "@/lib/groups";
import {
  parseRegisterSettingsValues,
  type RegisterSettingsValues,
} from "@/lib/register-settings";
import {
  parseSiteSettingsValues,
  type SiteSettingsValues,
} from "@/lib/site-settings";

export type SiteSettingsRead =
  | { status: "ok"; values: SiteSettingsValues }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getSiteSettings(): Promise<SiteSettingsRead> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  let response: Response;
  try {
    response = await fetch(`${backendUrl}/api/v1/admin/settings/site`, {
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
  const values = parseSiteSettingsValues(payload);

  if (values === null) {
    return { status: "failed" };
  }

  return { status: "ok", values };
}

export type RegisterSettingsRead =
  | { status: "ok"; values: RegisterSettingsValues; groups: AdminGroup[] }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getRegisterSettings(): Promise<RegisterSettingsRead> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  try {
    const [registerResponse, authResponse, groupsResponse] = await Promise.all([
      fetch(`${backendUrl}/api/v1/admin/settings/register`, {
        headers: { cookie: `session=${session}` },
        cache: "no-store",
      }),
      fetch(`${backendUrl}/api/v1/admin/settings/auth`, {
        headers: { cookie: `session=${session}` },
        cache: "no-store",
      }),
      fetch(`${backendUrl}/api/v1/admin/groups`, {
        headers: { cookie: `session=${session}` },
        cache: "no-store",
      }),
    ]);

    if (
      registerResponse.status === 403 ||
      authResponse.status === 403 ||
      groupsResponse.status === 403
    ) {
      return { status: "forbidden" };
    }

    if (!registerResponse.ok || !authResponse.ok) {
      return { status: "failed" };
    }

    const [registerPayload, authPayload, groupsPayload] = await Promise.all([
      registerResponse.json().catch(() => null),
      authResponse.json().catch(() => null),
      groupsResponse.json().catch(() => null),
    ]);
    const values = parseRegisterSettingsValues(registerPayload, authPayload);
    const groups = parseAdminGroups(groupsPayload);

    if (values === null || groups === null) {
      return { status: "failed" };
    }

    return { status: "ok", values, groups };
  } catch {
    return { status: "failed" };
  }
}
