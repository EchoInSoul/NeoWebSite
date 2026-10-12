export const THEME_STORAGE_KEY = "console-theme-mode";

export const CHANGE_EVENT_NAME = "neoblog-preference-change";

export const SYSTEM_THEME_ROUTES = ["/signin", "/signup"] as const;

export function isSystemThemeRoute(pathname: string): boolean {
  return SYSTEM_THEME_ROUTES.some(
    (route) => pathname === route || pathname.startsWith(`${route}/`),
  );
}

const DARK_COLOR_SCHEME_QUERY = "(prefers-color-scheme: dark)";

const SYSTEM_THEME_ROUTES_LITERAL = JSON.stringify([...SYSTEM_THEME_ROUTES]);

export const THEME_BOOTSTRAP_SCRIPT = `(function(){try{var p=location.pathname;var s=${SYSTEM_THEME_ROUTES_LITERAL}.some(function(r){return p===r||p.indexOf(r+"/")===0});var m=s?"system":localStorage.getItem("${THEME_STORAGE_KEY}")||"system";var d=m==="dark"||(m==="system"&&window.matchMedia("${DARK_COLOR_SCHEME_QUERY}").matches);document.documentElement.classList.toggle("dark",d)}catch(e){}})();`;
