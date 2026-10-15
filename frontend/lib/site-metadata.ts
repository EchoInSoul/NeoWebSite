function siteRecord(settings: unknown): Record<string, unknown> {
  const site = (settings as { site?: unknown } | null)?.site;

  if (typeof site !== "object" || site === null) {
    return {};
  }

  return site as Record<string, unknown>;
}

function textSetting(settings: unknown, key: string): string {
  const value = siteRecord(settings)[key];

  return typeof value === "string" ? value.trim() : "";
}

export function siteTitle(settings: unknown): string {
  return textSetting(settings, "title");
}

export function siteSubtitle(settings: unknown): string {
  return textSetting(settings, "subtitle");
}

export function composeSiteTitle(title: string, subtitle: string): string {
  const parts = [title.trim(), subtitle.trim()].filter((part) => part !== "");

  return parts.join(" - ");
}

export function siteDocumentTitle(settings: unknown): string {
  return composeSiteTitle(siteTitle(settings), siteSubtitle(settings));
}

export function siteDescription(settings: unknown): string {
  return textSetting(settings, "description");
}

export function siteKeywords(settings: unknown): string[] {
  const raw = textSetting(settings, "keywords");

  if (raw === "") {
    return [];
  }

  return raw
    .split(/[,，]/)
    .map((part) => part.trim())
    .filter((part) => part !== "");
}

export function siteBaseUrl(settings: unknown): URL | null {
  const raw = textSetting(settings, "url");

  if (raw === "") {
    return null;
  }

  try {
    const url = new URL(raw);

    return url.protocol === "http:" || url.protocol === "https:" ? url : null;
  } catch {
    return null;
  }
}

export function blocksSearchEngines(settings: unknown): boolean {
  return siteRecord(settings)["block_search_engines"] === true;
}
