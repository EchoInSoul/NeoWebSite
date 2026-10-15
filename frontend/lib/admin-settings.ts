import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";
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
