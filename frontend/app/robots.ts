import type { MetadataRoute } from "next";

import { getPublicSettings } from "@/lib/public-settings";
import { blocksSearchEngines } from "@/lib/site-metadata";

export default async function robots(): Promise<MetadataRoute.Robots> {
  const settings = await getPublicSettings();

  if (blocksSearchEngines(settings)) {
    return { rules: [{ userAgent: "*", disallow: "/" }] };
  }

  return { rules: [{ userAgent: "*", allow: "/", disallow: "/console" }] };
}
