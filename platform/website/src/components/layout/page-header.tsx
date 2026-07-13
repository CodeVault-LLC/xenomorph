import type * as React from "react"

import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"

type PageHeaderProps = {
  title: React.ReactNode
  kicker?: React.ReactNode
  description?: React.ReactNode
  actions?: React.ReactNode
  className?: string
}

export function PageHeader({
  title,
  kicker,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <header className={cn("flex flex-col gap-5", className)}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          {kicker ? (
            <div className="mb-3 flex size-10 items-center justify-center rounded-lg border border-border bg-card shadow-sm">
              {kicker}
            </div>
          ) : null}
          <h1 className="text-2xl font-semibold tracking-normal">{title}</h1>
          {description ? (
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {actions ? (
          <div className="flex shrink-0 items-center gap-3 text-sm text-muted-foreground">
            {actions}
          </div>
        ) : null}
      </div>
      <Separator />
    </header>
  )
}
