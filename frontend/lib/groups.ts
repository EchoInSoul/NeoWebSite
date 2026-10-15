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

export type PermissionMatrix = {
  actions: string[];
  rows: Array<{ resource: string; cells: PermissionMatrixCell[] }>;
};

const IMPLIED_ACTIONS: Record<string, string[]> = { edit: ["view"] };
const PRESET_ORDER = ["owner", "admin", "user"];
const RESOURCE_LABEL_KEYS: Record<string, string> = {
  "system.users": "resourceUsers",
  "system.groups": "resourceGroups",
  "system.settings": "resourceSettings",
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
