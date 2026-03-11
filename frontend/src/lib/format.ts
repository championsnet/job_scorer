export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms)) return "-";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60_000).toFixed(1)}m`;
}

export function formatDateTime(value?: string): string {
  if (!value) return "-";
  try {
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
      return "-";
    }
    // Firestore zero-value timestamps can come through as year 0001.
    if (parsed.getUTCFullYear() <= 1 || value.startsWith("0001-01-01")) {
      return "-";
    }
    return parsed.toLocaleString();
  } catch {
    return "-";
  }
}

export function formatPercent(value: number, total: number): string {
  if (!total) return "0%";
  return `${Math.round((value / total) * 100)}%`;
}

export function formatScore(value?: number): string {
  if (value == null) return "-";
  return value.toFixed(1);
}

export function compactNumber(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: "compact" }).format(value);
}
