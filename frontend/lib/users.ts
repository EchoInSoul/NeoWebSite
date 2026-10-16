export type AdminUserGroup = {
  id: string;
  systemKey: string | null;
  name: string;
};

export type AdminUser = {
  id: string;
  email: string;
  nickname: string;
  group: AdminUserGroup;
  extraPermissions: string[];
};

function parseUserGroup(value: unknown): AdminUserGroup | null {
  if (typeof value !== "object" || value === null) {
    return null;
  }

  const raw = value as Record<string, unknown>;

  if (typeof raw.id !== "string" || typeof raw.name !== "string") {
    return null;
  }

  const systemKey = raw.system_key;

  if (systemKey !== null && typeof systemKey !== "string") {
    return null;
  }

  return { id: raw.id, systemKey, name: raw.name };
}

export function parseAdminUser(payload: unknown): AdminUser | null {
  if (typeof payload !== "object" || payload === null) {
    return null;
  }

  const raw = payload as Record<string, unknown>;

  if (
    typeof raw.id !== "string" ||
    typeof raw.email !== "string" ||
    typeof raw.nickname !== "string" ||
    !Array.isArray(raw.extra_permissions) ||
    !raw.extra_permissions.every((permission) => typeof permission === "string")
  ) {
    return null;
  }

  const group = parseUserGroup(raw.group);

  if (group === null) {
    return null;
  }

  return {
    id: raw.id,
    email: raw.email,
    nickname: raw.nickname,
    group,
    extraPermissions: raw.extra_permissions as string[],
  };
}

export function parseAdminUsers(payload: unknown): AdminUser[] | null {
  const items = (payload as { items?: unknown } | null)?.items;

  if (!Array.isArray(items)) {
    return null;
  }

  const users: AdminUser[] = [];

  for (const item of items) {
    const user = parseAdminUser(item);

    if (user === null) {
      return null;
    }

    users.push(user);
  }

  return users;
}

export function isPrivilegedGroup(systemKey: string | null): boolean {
  return systemKey === "owner" || systemKey === "admin";
}

export type DeleteBlockReason = "deleteSelf" | "deletePrivileged";

export function deleteBlockReason(
  user: AdminUser,
  context: { currentUserEmail: string; viewerIsOwner: boolean },
): DeleteBlockReason | null {
  if (user.email === context.currentUserEmail) {
    return "deleteSelf";
  }

  if (isPrivilegedGroup(user.group.systemKey) && !context.viewerIsOwner) {
    return "deletePrivileged";
  }

  return null;
}

const ERROR_KEYS: Record<string, string> = {
  cannot_modify_self: "deleteSelf",
  owner_protected: "deletePrivileged",
  last_owner: "deleteLastOwner",
  forbidden: "forbidden",
};

export function userDeleteErrorKey(code: string): string {
  return ERROR_KEYS[code] ?? "deleteFailed";
}
