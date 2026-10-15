export type SiteSettingsValues = {
  title: string;
  subtitle: string;
  description: string;
  keywords: string;
  url: string;
  blockSearchEngines: boolean;
};

const SITE_SETTINGS_PATCH_KEYS: Record<keyof SiteSettingsValues, string> = {
  title: "title",
  subtitle: "subtitle",
  description: "description",
  keywords: "keywords",
  url: "url",
  blockSearchEngines: "block_search_engines",
};

export function parseSiteSettingsValues(
  payload: unknown,
): SiteSettingsValues | null {
  const values = (payload as { values?: unknown } | null)?.values;

  if (typeof values !== "object" || values === null) {
    return null;
  }

  const raw = values as Record<string, unknown>;

  if (
    typeof raw.title !== "string" ||
    typeof raw.subtitle !== "string" ||
    typeof raw.description !== "string" ||
    typeof raw.keywords !== "string" ||
    typeof raw.url !== "string" ||
    typeof raw.block_search_engines !== "boolean"
  ) {
    return null;
  }

  return {
    title: raw.title,
    subtitle: raw.subtitle,
    description: raw.description,
    keywords: raw.keywords,
    url: raw.url,
    blockSearchEngines: raw.block_search_engines,
  };
}

export function buildSiteSettingsPatch(
  values: SiteSettingsValues,
  baseline: SiteSettingsValues,
): Record<string, string | boolean> {
  const patch: Record<string, string | boolean> = {};
  const fields = Object.keys(
    SITE_SETTINGS_PATCH_KEYS,
  ) as (keyof SiteSettingsValues)[];

  for (const field of fields) {
    if (values[field] !== baseline[field]) {
      patch[SITE_SETTINGS_PATCH_KEYS[field]] = values[field];
    }
  }

  return patch;
}
