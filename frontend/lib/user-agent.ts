export type UserAgentDescription = {
  browser: string | null;
  os: string | null;
};

export function describeUserAgent(userAgent: string): UserAgentDescription {
  if (userAgent.trim().length === 0) {
    return { browser: null, os: null };
  }

  let browser: string | null = null;
  if (/\bEdg(?:e|A|iOS)?\//.test(userAgent)) {
    browser = withMajorVersion("Edge", userAgent, [/\bEdg(?:e|A|iOS)?\/(\d+)/]);
  } else if (/\bOPR\/|\bOpera\b/.test(userAgent)) {
    browser = withMajorVersion("Opera", userAgent, [
      /\bOPR\/(\d+)/,
      /\bOpera[ /](\d+)/,
    ]);
  } else if (/SamsungBrowser\//.test(userAgent)) {
    browser = withMajorVersion("Samsung Internet", userAgent, [
      /SamsungBrowser\/(\d+)/,
    ]);
  } else if (
    /\bFirefox\/|\bFxiOS\//.test(userAgent) &&
    !/Seamonkey/i.test(userAgent)
  ) {
    browser = withMajorVersion("Firefox", userAgent, [
      /\bFirefox\/(\d+)/,
      /\bFxiOS\/(\d+)/,
    ]);
  } else if (/\bChrome\/|\bCriOS\//.test(userAgent)) {
    browser = withMajorVersion("Chrome", userAgent, [
      /\bChrome\/(\d+)/,
      /\bCriOS\/(\d+)/,
    ]);
  } else if (/\bSafari\//.test(userAgent) && /Version\//.test(userAgent)) {
    browser = withMajorVersion("Safari", userAgent, [/Version\/(\d+)/]);
  }

  let os: string | null = null;
  if (/Windows NT|Windows Phone/.test(userAgent)) {
    os = "Windows";
  } else if (/CrOS/.test(userAgent)) {
    os = "ChromeOS";
  } else if (/Android/.test(userAgent)) {
    os = "Android";
  } else if (/iPhone|iPad|iPod/.test(userAgent)) {
    os = "iOS";
  } else if (/Mac OS X|Macintosh/.test(userAgent)) {
    os = "macOS";
  } else if (/Linux/.test(userAgent)) {
    os = "Linux";
  }

  return { browser, os };
}

function withMajorVersion(
  baseName: string,
  userAgent: string,
  versionPatterns: readonly RegExp[],
): string {
  for (const versionPattern of versionPatterns) {
    const match = userAgent.match(versionPattern);
    if (match?.[1] !== undefined) {
      return `${baseName} ${match[1]}`;
    }
  }
  return baseName;
}
