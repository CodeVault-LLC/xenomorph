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

export const MetricCard = ({
  icon: Icon,
  label,
  value,
  tone = "default",
  className,
}: MetricCardProps) => {
  return (
    <Card className={cn(className)}>
      <CardHeader>
        <CardDescription className="flex items-center gap-2 text-base">
          <Icon className="size-5" />
          {label}
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-3 pb-4">
        <CardTitle className={cn("text-md", metricToneClass[tone])}>
          {value}
        </CardTitle>
      </CardContent>
    </Card>
  )
}
