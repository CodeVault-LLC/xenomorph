export type MetricTone = "default" | "good" | "warn" | "danger"

export const metricToneClass: Record<MetricTone, string> = {
  default: "text-foreground",
  good: "text-emerald-700 dark:text-emerald-400",
  warn: "text-amber-700 dark:text-amber-400",
  danger: "text-rose-700 dark:text-rose-400",
}
