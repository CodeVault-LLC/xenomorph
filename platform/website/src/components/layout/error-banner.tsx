import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
    <Alert variant="destructive" className={cn(className)}>
      <AlertTitle>Gateway request failed</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}
