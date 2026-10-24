const SIGNUP_SUCCESS_KEY = "neoblog.signup.success";
const SIGNUP_PENDING_KEY = "neoblog.signup.pending";

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

export function markSignupPending() {
  window.sessionStorage.setItem(SIGNUP_PENDING_KEY, "1");
}

export function consumeSignupPending() {
  const marked = window.sessionStorage.getItem(SIGNUP_PENDING_KEY) === "1";
  if (marked) {
    window.sessionStorage.removeItem(SIGNUP_PENDING_KEY);
  }
  return marked;
}
