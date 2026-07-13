import { Wifi, WifiOff } from "lucide-react"

import { Badge } from "@/components/ui/badge"

export function StatusBadge({ online }: { online: boolean }) {
  return (
    <Badge
      variant={online ? "online" : "offline"}
      className="items-center gap-2"
    >
      {online ? <Wifi className="size-5" /> : <WifiOff className="size-5" />}
      {online ? "ONLINE" : "OFFLINE"}
    </Badge>
  )
}
