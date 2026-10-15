import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { getLocale } from "next-intl/server";

import { ThemeSync } from "@/components/theme-sync";
import { Toaster } from "@/components/ui/sonner";
import { THEME_BOOTSTRAP_SCRIPT } from "@/lib/bootstrap-scripts";
import { getPublicSettings } from "@/lib/public-settings";
import {
  blocksSearchEngines,
  siteBaseUrl,
  siteDescription,
  siteDocumentTitle,
  siteKeywords,
  siteTitle,
} from "@/lib/site-metadata";

import "./globals.css";

const geistSans = Geist({
  variable: "--font-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-mono",
  subsets: ["latin"],
});

export async function generateMetadata(): Promise<Metadata> {
  const settings = await getPublicSettings();
  const title = siteDocumentTitle(settings);
  const siteName = siteTitle(settings);
  const description = siteDescription(settings);
  const keywords = siteKeywords(settings);
  const baseUrl = siteBaseUrl(settings);
  const openGraph = {
    ...(title === "" ? {} : { title }),
    ...(description === "" ? {} : { description }),
    ...(siteName === "" ? {} : { siteName }),
  };

  return {
    ...(title === "" ? {} : { title }),
    ...(description === "" ? {} : { description }),
    ...(keywords.length === 0 ? {} : { keywords }),
    ...(baseUrl === null ? {} : { metadataBase: baseUrl }),
    ...(blocksSearchEngines(settings) ? { robots: { index: false } } : {}),
    ...(Object.keys(openGraph).length === 0 ? {} : { openGraph }),
  };
}

export const instant = false;

export default async function RootLayout({ children }: LayoutProps<"/">) {
  const locale = await getLocale();

  return (
    <html
      lang={locale}
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col">
        <script dangerouslySetInnerHTML={{ __html: THEME_BOOTSTRAP_SCRIPT }} />
        <ThemeSync />
        <NextIntlClientProvider>{children}</NextIntlClientProvider>
        <Toaster />
      </body>
    </html>
  );
}
