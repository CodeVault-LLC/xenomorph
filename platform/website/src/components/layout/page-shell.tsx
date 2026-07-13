import * as React from "react"

import { cn } from "@/lib/utils"

type PageShellProps = {
  className?: string
  children: React.ReactNode
}

export function PageShell({ className, children }: PageShellProps) {
  return (
    <main className="min-h-[90vh] bg-background px-4 py-6 text-foreground sm:px-6 lg:px-8">
      <div
        className={cn(
          "mx-auto flex w-full max-w-7xl flex-col gap-5",
          className
        )}
      >
        {children}
      </div>
    </main>
  )
}
