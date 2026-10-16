const SIGNUP_SUCCESS_KEY = "neoblog.signup.success";

export function markSignupSuccess() {
  window.sessionStorage.setItem(SIGNUP_SUCCESS_KEY, "1");
}

export function consumeSignupSuccess() {
  const marked = window.sessionStorage.getItem(SIGNUP_SUCCESS_KEY) === "1";
  if (marked) {
    window.sessionStorage.removeItem(SIGNUP_SUCCESS_KEY);
  }
  return marked;
}
