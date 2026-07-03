import type * as React from "react"

import { cn } from "@/lib/utils"

type FactProps = {
  label?: string
  value: React.ReactNode
  mono?: boolean
  icon?: React.ComponentType<{ className?: string }>
  variant?: "grid" | "row"
  className?: string
}

export function Fact({
  label,
  value,
  mono,
  icon: Icon,
  variant = "grid",
  className,
}: FactProps) {
  if (variant === "row" || Icon) {
    return (
      <div
        className={cn(
          "flex gap-3",
          variant === "grid" ? "items-center" : "p-4",
          className
        )}
      >
        {Icon ? <Icon className="mt-0.5 size-4 text-muted-foreground" /> : null}
        <div className="min-w-0">
          {label ? (
            <div className="text-xs font-medium text-muted-foreground uppercase">
              {label}
            </div>
          ) : null}
          <div className={cn(mono ? "font-mono text-xs break-all" : "text-sm")}>
            {value}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className={cn("grid gap-1", className)}>
      {label ? (
        <div className="text-xs font-medium text-muted-foreground uppercase">
          {label}
        </div>
      ) : null}
      <div className={cn(mono ? "font-mono text-xs break-all" : "text-sm")}>
        {value}
      </div>
    </div>
  )
}
