import type * as React from "react"

import {
  type MetricTone,
  metricToneClass,
} from "@/components/dashboard/metric-tone"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
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
    <Card className={cn(className)}>
      <CardHeader className="pb-2">
        <CardDescription className="flex items-center gap-2">
          <Icon />
          {label}
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-0">
        <CardTitle className={cn("text-2xl", metricToneClass[tone])}>
          {value}
        </CardTitle>
      </CardContent>
    </Card>
  )
}
