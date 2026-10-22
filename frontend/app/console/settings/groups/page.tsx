import { getTranslations } from "next-intl/server";

import { getAdminGroups, getPermissionCatalog } from "@/lib/admin-groups";
import { getAuthenticatedUser } from "@/lib/auth-session";
import { buildPermissionMatrix } from "@/lib/groups";

import { GroupsPanel } from "./groups-panel";

export const instant = false;

export default async function SettingsGroupsPage() {
  const t = await getTranslations("groups");
  const [groupsRead, catalogRead] = await Promise.all([
    getAdminGroups(),
    getPermissionCatalog(),
  ]);
  const user = await getAuthenticatedUser().catch(() => null);
  const canEdit = user?.permissions.includes("system.groups.edit") ?? false;

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("title")}</h1>
      {groupsRead.status === "ok" && catalogRead.status === "ok" ? (
        <GroupsPanel
          initialGroups={groupsRead.groups}
          matrix={buildPermissionMatrix(catalogRead.catalog)}
          canEdit={canEdit}
        />
      ) : (
        <p className="text-sm text-muted-foreground">
          {groupsRead.status === "forbidden"
            ? t("noPermission")
            : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
