import { Wifi, WifiOff } from "lucide-react"

import { Badge } from "@/components/ui/badge"

export function StatusBadge({ online }: { online: boolean }) {
  return (
    <Badge variant={online ? "online" : "offline"}>
      {online ? (
        <Wifi className="mr-1 size-3" />
      ) : (
        <WifiOff className="mr-1 size-3" />
      )}
      {online ? "ONLINE" : "OFFLINE"}
    </Badge>
  )
}
