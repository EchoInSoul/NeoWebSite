import { getTranslations } from "next-intl/server";

import { GroupsPanel } from "./groups-panel";

export default async function SettingsGroupsPage() {
  const t = await getTranslations("groups");

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("title")}</h1>
      <GroupsPanel />
    </div>
  );
}
