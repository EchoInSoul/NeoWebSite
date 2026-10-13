import { getTranslations } from "next-intl/server";

import { getAuthenticatedUser } from "@/lib/auth-session";
import { ProfileForm } from "./profile-form";

export default async function PersonalInfoPage() {
  const t = await getTranslations("uc");
  const user = await getAuthenticatedUser().catch(() => null);

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">
        {t("personalInfo")}
      </h1>
      <ProfileForm profile={user} />
    </div>
  );
}
