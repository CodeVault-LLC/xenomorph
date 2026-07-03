import { cn } from "@/lib/utils"

export function ErrorBanner({
  message,
  className,
}: {
  message: string | null
  className?: string
}) {
  if (!message) {
    return null
  }

  return (
    <div
      role="alert"
      className={cn(
        "rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive",
        className
      )}
    >
      {message}
    </div>
  )
}
