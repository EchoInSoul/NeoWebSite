const RESET_REQUESTED_KEY = "neoblog.passwordReset.requested";
const RESET_DONE_KEY = "neoblog.passwordReset.done";

function consume(key: string): boolean {
  const marked = window.sessionStorage.getItem(key) === "1";
  if (marked) {
    window.sessionStorage.removeItem(key);
  }
  return marked;
}

export function markResetRequested() {
  window.sessionStorage.setItem(RESET_REQUESTED_KEY, "1");
}

export function consumeResetRequested() {
  return consume(RESET_REQUESTED_KEY);
}

export function markResetDone() {
  window.sessionStorage.setItem(RESET_DONE_KEY, "1");
}

export function consumeResetDone() {
  return consume(RESET_DONE_KEY);
}
