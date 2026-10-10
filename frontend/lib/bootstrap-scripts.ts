export const THEME_STORAGE_KEY = "console-theme-mode";

export const CHANGE_EVENT_NAME = "neoblog-preference-change";

const DARK_COLOR_SCHEME_QUERY = "(prefers-color-scheme: dark)";

export const THEME_BOOTSTRAP_SCRIPT = `(function(){try{var m=localStorage.getItem("${THEME_STORAGE_KEY}")||"system";var d=m==="dark"||(m==="system"&&window.matchMedia("${DARK_COLOR_SCHEME_QUERY}").matches);document.documentElement.classList.toggle("dark",d)}catch(e){}})();`;
