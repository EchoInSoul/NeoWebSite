import { getTranslations } from "next-intl/server";

import { getAdminUsers } from "@/lib/admin-users";
import { getAuthenticatedUser } from "@/lib/auth-session";

import { UsersPanel } from "./users-panel";

export const instant = false;

export default async function SettingsUsersPage() {
  const t = await getTranslations("users");
  const [usersRead, user] = await Promise.all([
    getAdminUsers(),
    getAuthenticatedUser().catch(() => null),
  ]);
  const canDelete = user?.permissions.includes("system.users.edit") ?? false;

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("title")}</h1>
      {usersRead.status === "ok" ? (
        <UsersPanel
          initialUsers={usersRead.users}
          currentUserEmail={user?.email ?? ""}
          viewerIsOwner={user?.group.systemKey === "owner"}
          canDelete={canDelete}
        />
      ) : (
        <p className="text-sm text-muted-foreground">
          {usersRead.status === "forbidden"
            ? t("noPermission")
            : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
