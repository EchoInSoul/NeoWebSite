import { cookies } from "next/headers";

import { backendUrl } from "@/lib/backend-url";
import {
  parseAdminGroups,
  parsePermissionCatalog,
  type AdminGroup,
  type PermissionCatalogItem,
} from "@/lib/groups";

export type AdminGroupsRead =
  | { status: "ok"; groups: AdminGroup[] }
  | { status: "forbidden" }
  | { status: "failed" };

export async function getAdminGroups(): Promise<AdminGroupsRead> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  let response: Response;
  try {
    response = await fetch(`${backendUrl}/api/v1/admin/groups`, {
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
  const groups = parseAdminGroups(payload);

  if (groups === null) {
    return { status: "failed" };
  }

  return { status: "ok", groups };
}

export type PermissionCatalogRead =
  { status: "ok"; catalog: PermissionCatalogItem[] } | { status: "failed" };

export async function getPermissionCatalog(): Promise<PermissionCatalogRead> {
  const session = (await cookies()).get("session")?.value;

  if (!session) {
    return { status: "failed" };
  }

  let response: Response;
  try {
    response = await fetch(`${backendUrl}/api/v1/admin/permissions`, {
      headers: { cookie: `session=${session}` },
      cache: "no-store",
    });
  } catch {
    return { status: "failed" };
  }

  if (!response.ok) {
    return { status: "failed" };
  }

  const payload = await response.json().catch(() => null);
  const catalog = parsePermissionCatalog(payload);

  if (catalog === null) {
    return { status: "failed" };
  }

  return { status: "ok", catalog };
}
