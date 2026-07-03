import type * as React from "react"

import {
  type MetricTone,
  metricToneClass,
} from "@/components/dashboard/metric-tone"
import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"

type MetricCardProps = {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: string
  tone?: MetricTone
  className?: string
}

export function MetricCard({
  icon: Icon,
  label,
  value,
  tone = "default",
  className,
}: MetricCardProps) {
  return (
    <Card className={cn("p-4", className)}>
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Icon className="size-4" />
        {label}
      </div>
      <div className={cn("mt-3 text-2xl font-semibold", metricToneClass[tone])}>
        {value}
      </div>
    </Card>
  )
}
