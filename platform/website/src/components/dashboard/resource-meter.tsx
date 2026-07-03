import { cn } from "@/lib/utils"

type ResourceMeterProps = {
  label: string
  value: number
  detail?: string
}

export function ResourceMeter({ label, value, detail }: ResourceMeterProps) {
  const pct = Math.round(Math.max(0, Math.min(1, value)) * 100)
  const tone =
    pct >= 85 ? "bg-rose-500" : pct >= 70 ? "bg-amber-500" : "bg-emerald-500"

  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="text-sm font-medium">{label}</div>
          {detail ? (
            <div className="mt-1 text-xs text-muted-foreground">{detail}</div>
          ) : null}
        </div>
        <div className="text-2xl font-semibold">{pct}%</div>
      </div>
      <div className="mt-4 h-2 rounded-full bg-muted">
        <div
          className={cn("h-full rounded-full transition-all", tone)}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}
