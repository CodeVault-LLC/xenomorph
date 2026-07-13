export type MetricTone = "default" | "good" | "warn" | "danger"

export const metricToneClass: Record<MetricTone, string> = {
  default: "text-foreground",
  good: "text-primary",
  warn: "text-foreground",
  danger: "text-destructive",
}
