const SIGNIN_SUCCESS_KEY = "neoblog.signin.success";

export function markSigninSuccess() {
  window.sessionStorage.setItem(SIGNIN_SUCCESS_KEY, "1");
}

export function consumeSigninSuccess() {
  const marked = window.sessionStorage.getItem(SIGNIN_SUCCESS_KEY) === "1";
  if (marked) {
    window.sessionStorage.removeItem(SIGNIN_SUCCESS_KEY);
  }
  return marked;
}
