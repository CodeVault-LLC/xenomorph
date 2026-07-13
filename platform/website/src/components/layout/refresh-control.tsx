import { RefreshCw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type RefreshControlProps = {
  updatedAt: Date | null
  loading: boolean
  onRefresh: () => void
  format: (value: Date) => string
  className?: string
}

export function RefreshControl({
  updatedAt,
  loading,
  onRefresh,
  format,
  className,
}: RefreshControlProps) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <span>{updatedAt ? `Updated ${format(updatedAt)}` : "Not updated"}</span>
      <Button variant="outline" onClick={onRefresh} disabled={loading}>
        <RefreshCw
          className={loading ? "animate-spin" : ""}
          data-icon="inline-start"
        />
        Refresh
      </Button>
    </div>
  )
}
