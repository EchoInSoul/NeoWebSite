import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";
import {
  parseCaptchaSettingsValues,
  type CaptchaSettingsValues,
} from "@/lib/captcha-settings";
import { parseAdminGroups } from "@/lib/groups";
import type { AdminGroup } from "@/lib/groups";
import {
  parseLoginGroupValues,
  type LoginSettingsValues,
} from "@/lib/login-settings";
import {
  parseRegisterSettingsValues,
  type RegisterSettingsValues,
} from "@/lib/register-settings";
import {
  parseSiteSettingsValues,
  type SiteSettingsValues,
} from "@/lib/site-settings";

type AdminSettingsRead<T> =
  { status: "ok"; values: T } | { status: "forbidden" } | { status: "failed" };

type AdminPayload =
  | { status: "ok"; payload: unknown }
  | { status: "forbidden" }
  | { status: "failed" };

async function fetchAdminPayload(path: string): Promise<AdminPayload> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  try {
    const response = await fetch(`${backendUrl}${path}`, {
      headers: { cookie: `session=${session}` },
      cache: "no-store",
    });

    if (response.status === 403) {
      return { status: "forbidden" };
    }

    if (!response.ok) {
      return { status: "failed" };
    }

    return { status: "ok", payload: await response.json().catch(() => null) };
  } catch {
    return { status: "failed" };
  }
}

function toAdminSettingsRead<T>(
  read: AdminPayload,
  parse: (payload: unknown) => T | null,
): AdminSettingsRead<T> {
  if (read.status !== "ok") {
    return { status: read.status };
  }

  const values = parse(read.payload);

  return values === null ? { status: "failed" } : { status: "ok", values };
}

export type SiteSettingsRead = AdminSettingsRead<SiteSettingsValues>;

export async function getSiteSettings(): Promise<SiteSettingsRead> {
  return toAdminSettingsRead(
    await fetchAdminPayload("/api/v1/admin/settings/site"),
    parseSiteSettingsValues,
  );
}

export type RegisterSettingsRead =
  | { status: "ok"; values: RegisterSettingsValues; groups: AdminGroup[] }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getRegisterSettings(): Promise<RegisterSettingsRead> {
  const [register, auth, groups] = await Promise.all([
    fetchAdminPayload("/api/v1/admin/settings/register"),
    fetchAdminPayload("/api/v1/admin/settings/auth"),
    fetchAdminPayload("/api/v1/admin/groups"),
  ]);

  if (
    register.status !== "ok" ||
    auth.status !== "ok" ||
    groups.status !== "ok"
  ) {
    return {
      status:
        register.status === "forbidden" ||
        auth.status === "forbidden" ||
        groups.status === "forbidden"
          ? "forbidden"
          : "failed",
    };
  }

  const values = parseRegisterSettingsValues(register.payload, auth.payload);
  const adminGroups = parseAdminGroups(groups.payload);

  if (values === null || adminGroups === null) {
    return { status: "failed" };
  }

  return { status: "ok", values, groups: adminGroups };
}

export type LoginSettingsRead = AdminSettingsRead<LoginSettingsValues>;

export async function getLoginSettings(): Promise<LoginSettingsRead> {
  return toAdminSettingsRead(
    await fetchAdminPayload("/api/v1/admin/settings/auth"),
    parseLoginGroupValues,
  );
}

export type CaptchaSettingsRead = AdminSettingsRead<CaptchaSettingsValues>;

export async function getCaptchaSettings(): Promise<CaptchaSettingsRead> {
  return toAdminSettingsRead(
    await fetchAdminPayload("/api/v1/admin/settings/auth"),
    parseCaptchaSettingsValues,
  );
}
