import { Wifi, WifiOff } from "lucide-react"

import { Badge } from "@/components/ui/badge"

export function StatusBadge({ online }: { online: boolean }) {
  return (
    <Badge variant={online ? "online" : "offline"}>
      {online ? <Wifi /> : <WifiOff />}
      {online ? "ONLINE" : "OFFLINE"}
    </Badge>
  )
}
