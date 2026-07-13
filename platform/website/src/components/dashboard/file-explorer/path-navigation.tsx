import { ChevronRight, HardDrive } from "lucide-react"

import { Button } from "@/components/ui/button"

export function PathNavigation({
  relativePath,
  onNavigate,
}: {
  relativePath: string
  onNavigate: (path: string) => void
}) {
  const parts = relativePath ? relativePath.split("/") : []

  return (
    <nav
      className="flex flex-wrap items-center gap-1"
      aria-label="Current filesystem path"
    >
      <Button variant="ghost" size="sm" onClick={() => onNavigate("")}>
        <HardDrive data-icon="inline-start" /> Root
      </Button>
      {parts.map((part, index) => (
        <div key={`${part}-${index}`} className="flex items-center gap-1">
          <ChevronRight aria-hidden="true" />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onNavigate(parts.slice(0, index + 1).join("/"))}
          >
            {part}
          </Button>
        </div>
      ))}
    </nav>
  )
}
