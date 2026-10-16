export type AdminGroup = {
  id: string;
  systemKey: string | null;
  name: string;
  isPreset: boolean;
  memberCount: number;
  permissions: string[];
};

export type PermissionCatalogItem = { key: string; name: string };

export type PermissionMatrixCell = { action: string; key: string } | null;

export type PermissionMatrixRow = {
  resource: string;
  cells: PermissionMatrixCell[];
};

export type PermissionMatrix = {
  actions: string[];
  rows: PermissionMatrixRow[];
};

export function parseAdminGroup(payload: unknown): AdminGroup | null {
  if (typeof payload !== "object" || payload === null) {
    return null;
  }

  const raw = payload as Record<string, unknown>;

  if (
    typeof raw.id !== "string" ||
    typeof raw.name !== "string" ||
    typeof raw.is_preset !== "boolean" ||
    typeof raw.member_count !== "number" ||
    !Array.isArray(raw.permissions) ||
    !raw.permissions.every((permission) => typeof permission === "string")
  ) {
    return null;
  }

  const systemKey = raw.system_key;

  if (systemKey !== null && typeof systemKey !== "string") {
    return null;
  }

  return {
    id: raw.id,
    systemKey,
    name: raw.name,
    isPreset: raw.is_preset,
    memberCount: raw.member_count,
    permissions: raw.permissions as string[],
  };
}

export function parseAdminGroups(payload: unknown): AdminGroup[] | null {
  const items = (payload as { items?: unknown } | null)?.items;

  if (!Array.isArray(items)) {
    return null;
  }

  const groups: AdminGroup[] = [];

  for (const item of items) {
    const group = parseAdminGroup(item);

    if (group === null) {
      return null;
    }

    groups.push(group);
  }

  return groups;
}

export function parsePermissionCatalog(
  payload: unknown,
): PermissionCatalogItem[] | null {
  const items = (payload as { items?: unknown } | null)?.items;

  if (!Array.isArray(items)) {
    return null;
  }

  const catalog: PermissionCatalogItem[] = [];

  for (const item of items) {
    const raw = item as { key?: unknown; name?: unknown } | null;

    if (typeof raw?.key !== "string" || typeof raw.name !== "string") {
      return null;
    }

    catalog.push({ key: raw.key, name: raw.name });
  }

  return catalog;
}

const IMPLIED_ACTIONS: Record<string, string[]> = { edit: ["view"] };
const PRESET_ORDER = ["owner", "admin", "user"];
const RESOURCE_LABEL_KEYS: Record<string, string> = {
  "system.users": "resourceUsers",
  "system.groups": "resourceGroups",
  "system.settings": "resourceGeneral",
  "system.register": "resourceRegister",
};
const ACTION_LABEL_KEYS: Record<string, string> = {
  view: "actionView",
  edit: "actionEdit",
};

export function buildPermissionMatrix(
  catalog: PermissionCatalogItem[],
): PermissionMatrix {
  const actions: string[] = [];
  const cellsByResource = new Map<string, Map<string, string>>();

  for (const item of catalog) {
    const separator = item.key.lastIndexOf(".");
    const resource = item.key.slice(0, separator);
    const action = item.key.slice(separator + 1);

    if (!actions.includes(action)) {
      actions.push(action);
    }

    const cells = cellsByResource.get(resource) ?? new Map<string, string>();
    cells.set(action, item.key);
    cellsByResource.set(resource, cells);
  }

  const rows = Array.from(cellsByResource, ([resource, cells]) => ({
    resource,
    cells: actions.map((action) => {
      const key = cells.get(action);
      return key === undefined ? null : { action, key };
    }),
  }));

  return { actions, rows };
}

export function resourceLabelKey(resource: string): string | null {
  return RESOURCE_LABEL_KEYS[resource] ?? null;
}

export function actionLabelKey(action: string): string | null {
  return ACTION_LABEL_KEYS[action] ?? null;
}

// 矩阵显示分组照侧栏（labelKey 取 sidebar 命名空间）；system.users 暂不在矩阵显示，键仍在后端目录
const MATRIX_GROUPS: Array<{ labelKey: string; resources: string[] }> = [
  { labelKey: "settings", resources: ["system.settings"] },
  { labelKey: "signInAndRegister", resources: ["system.register"] },
  { labelKey: "users", resources: ["system.groups"] },
];

const DISPLAYED_RESOURCES = new Set(
  MATRIX_GROUPS.flatMap((group) => group.resources),
);

export type MatrixSection = {
  labelKey: string;
  rows: PermissionMatrixRow[];
};

export function buildMatrixSections(matrix: PermissionMatrix): MatrixSection[] {
  const rowsByResource = new Map(matrix.rows.map((row) => [row.resource, row]));

  return MATRIX_GROUPS.map((group) => ({
    labelKey: group.labelKey,
    rows: group.resources
      .map((resource) => rowsByResource.get(resource))
      .filter((row): row is PermissionMatrixRow => row !== undefined),
  })).filter((section) => section.rows.length > 0);
}

export function isDisplayedPermission(permission: string): boolean {
  const separator = permission.lastIndexOf(".");

  if (separator < 0) {
    return false;
  }

  return DISPLAYED_RESOURCES.has(permission.slice(0, separator));
}

export function applyPermissionToggle(
  selected: string[],
  key: string,
  checked: boolean,
): string[] {
  const next = new Set(selected);
  const separator = key.lastIndexOf(".");
  const resource = key.slice(0, separator);
  const action = key.slice(separator + 1);

  if (checked) {
    next.add(key);
    for (const implied of IMPLIED_ACTIONS[action] ?? []) {
      next.add(`${resource}.${implied}`);
    }
  } else {
    next.delete(key);
    for (const [candidate, impliedActions] of Object.entries(IMPLIED_ACTIONS)) {
      if (impliedActions.includes(action)) {
        next.delete(`${resource}.${candidate}`);
      }
    }
  }

  return Array.from(next);
}

export function togglePermissionColumn(
  selected: string[],
  keys: string[],
  checked: boolean,
): string[] {
  return keys.reduce(
    (accumulator, key) => applyPermissionToggle(accumulator, key, checked),
    selected,
  );
}

export function permissionColumnState(
  selected: string[],
  keys: string[],
): "all" | "some" | "none" {
  if (keys.length === 0) {
    return "none";
  }

  const count = keys.filter((key) => selected.includes(key)).length;

  if (count === 0) {
    return "none";
  }

  return count === keys.length ? "all" : "some";
}

export function samePermissionSet(left: string[], right: string[]): boolean {
  const leftSet = new Set(left);
  const rightSet = new Set(right);

  if (leftSet.size !== rightSet.size) {
    return false;
  }

  return Array.from(leftSet).every((value) => rightSet.has(value));
}

export function sortGroupsForDisplay(groups: AdminGroup[]): AdminGroup[] {
  const presetIndex = (group: AdminGroup): number => {
    if (group.systemKey === null) {
      return PRESET_ORDER.length;
    }
    const index = PRESET_ORDER.indexOf(group.systemKey);
    return index === -1 ? PRESET_ORDER.length : index;
  };

  const presets = groups.filter((group) => group.isPreset);
  const others = groups.filter((group) => !group.isPreset);
  presets.sort((left, right) => presetIndex(left) - presetIndex(right));

  return [...presets, ...others];
}

const ERROR_KEYS: Record<string, string> = {
  preset_group_locked: "presetLocked",
  group_name_taken: "nameTaken",
  group_not_empty: "notEmpty",
  grant_forbidden: "grantForbidden",
  forbidden: "forbidden",
  not_found: "groupMissing",
};

const FALLBACK_KEYS: Record<"create" | "edit" | "delete", string> = {
  create: "createFailed",
  edit: "saveFailed",
  delete: "deleteFailed",
};

export function groupErrorKey(
  code: string,
  context: "create" | "edit" | "delete",
): string {
  return ERROR_KEYS[code] ?? FALLBACK_KEYS[context];
}
