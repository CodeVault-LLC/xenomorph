import { LoaderCircle, RefreshCw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type RefreshControlProps = {
  updatedAt: Date | null
  loading: boolean
  onRefresh: () => void | Promise<unknown>
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
      <span className="text-sm text-muted-foreground">
        {updatedAt ? `Updated ${format(updatedAt)}` : "Waiting for data"}
      </span>
      <Button variant="outline" onClick={onRefresh} disabled={loading}>
        {loading ? (
          <LoaderCircle className="animate-spin" data-icon="inline-start" />
        ) : (
          <RefreshCw data-icon="inline-start" />
        )}
        Refresh
      </Button>
    </div>
  )
}
