type SessionExpiryListener = () => void;

const listeners = new Set<SessionExpiryListener>();
let didReport = false;

export function reportSessionExpired(): void {
  if (typeof window === "undefined" || didReport || listeners.size === 0) {
    return;
  }

  didReport = true;
  for (const listener of listeners) {
    listener();
  }
}

export function subscribeSessionExpired(
  listener: SessionExpiryListener,
): () => void {
  listeners.add(listener);
  didReport = false;

  return () => {
    listeners.delete(listener);
  };
}
