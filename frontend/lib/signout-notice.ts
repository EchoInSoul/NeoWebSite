const SIGNOUT_SUCCESS_KEY = "neoblog.signout.success";

export function markSignoutSuccess() {
  window.sessionStorage.setItem(SIGNOUT_SUCCESS_KEY, "1");
}

export function consumeSignoutSuccess() {
  const marked = window.sessionStorage.getItem(SIGNOUT_SUCCESS_KEY) === "1";
  if (marked) {
    window.sessionStorage.removeItem(SIGNOUT_SUCCESS_KEY);
  }
  return marked;
}
