export function parseRetryAfter(value: string | null): number | undefined {
  if (!value) {
    return undefined;
  }

  const seconds = Number(value);
  return Number.isInteger(seconds) && seconds > 0 ? seconds : undefined;
}

export function formatRetryDuration(seconds: number, locale: string): string {
  const safe = Math.max(1, Math.ceil(seconds));
  const minutes = Math.floor(safe / 60);
  const rest = safe % 60;
  const chinese = locale.toLowerCase().startsWith("zh");

  if (minutes === 0) {
    return chinese ? `${safe} 秒` : `${safe}s`;
  }
  if (rest === 0) {
    return chinese ? `${minutes} 分钟` : `${minutes}m`;
  }
  return chinese ? `${minutes} 分 ${rest} 秒` : `${minutes}m ${rest}s`;
}
