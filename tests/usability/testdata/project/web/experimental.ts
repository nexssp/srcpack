export function experimentalRetryDelay(attempt: number): number {
  const boundedAttempt = Math.max(0, Math.min(attempt, 8));
  const jitter = Math.random() * 125;
  return 250 * 2 ** boundedAttempt + jitter;
}

export function shouldRetry(status: number): boolean {
  return status === 408 || status === 429 || status >= 500;
}
