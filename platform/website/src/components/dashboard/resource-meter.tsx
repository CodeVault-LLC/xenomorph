import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@/components/ui/progress"

type ResourceMeterProps = {
  label: string
  value: number
  detail?: string
}

export function ResourceMeter({ label, value, detail }: ResourceMeterProps) {
  const pct = Math.round(Math.max(0, Math.min(1, value)) * 100)

  return (
    <Progress value={pct} className="rounded-lg border border-border p-4">
      <div className="min-w-0">
        <ProgressLabel>{label}</ProgressLabel>
        {detail ? (
          <div className="mt-1 text-xs text-muted-foreground">{detail}</div>
        ) : null}
      </div>
      <ProgressValue>{pct}%</ProgressValue>
    </Progress>
  )
}
