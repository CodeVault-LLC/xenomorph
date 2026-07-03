import { Badge } from "@/components/ui/badge"
import { detectOS, display } from "@/lib/clients"

export function OSBadge({
  value,
  detailed = false,
}: {
  value: string
  detailed?: boolean
}) {
  const os = detectOS(value)
  const Icon = os.icon

  return (
    <Badge variant="outline">
      {os.variant === "arch" ? (
        <ArchMark className="mr-1 size-3" />
      ) : (
        <Icon className="mr-1 size-3" />
      )}
      {detailed ? os.label : os.family}
    </Badge>
  )
}

export function OSLabel({ value }: { value: string }) {
  const os = detectOS(value)
  const Icon = os.icon

  return (
    <div className="flex items-center gap-2">
      <span className="flex size-7 items-center justify-center rounded-md border border-border bg-background">
        {os.variant === "arch" ? (
          <ArchMark className="size-4 text-sky-600" />
        ) : (
          <Icon className="size-4 text-muted-foreground" />
        )}
      </span>
      <div>
        <div className="font-medium">{os.label}</div>
        <div className="text-xs text-muted-foreground">{display(value)}</div>
      </div>
    </div>
  )
}

function ArchMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={className}
      fill="currentColor"
    >
      <path d="M12 2 3.6 21.5c2.1-1.3 4.1-2.1 6-2.4l1.3-3.4c-1.3-.3-2.3-.8-3-1.4 1.2.4 2.5.6 3.8.5l1.7-4.4 2.7 7.1c-1.1-.8-2.3-1.4-3.6-1.8l-1.4 3.5c3.3.1 6.4 1.1 9.3 2.9L12 2Z" />
    </svg>
  )
}
