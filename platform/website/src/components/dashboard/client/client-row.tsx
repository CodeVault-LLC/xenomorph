import { Link } from "@tanstack/react-router"

import { OSLabel } from "@/components/dashboard/os-badge"
import { StatusBadge } from "@/components/dashboard/status-badge"
import { Button } from "@/components/ui/button"
import {
  display,
  formatDate,
  formatIP,
  formatPercent,
  formatRelative,
  type ClientSnapshot,
} from "@/lib/clients"
import { TableCell, TableRow } from "@/components/ui/table"
import { ChevronRight } from "lucide-react"

export const ClientRow = ({ client }: { client: ClientSnapshot }) => {
  return (
    <TableRow className="group hover:bg-muted/50">
      <TableCell>
        <StatusBadge online={client.is_online} />
      </TableCell>
      <TableCell className="font-medium">
        <Button
          render={
            <Link to="/agents/$agentId" params={{ agentId: client.agent_id }} />
          }
          nativeButton={false}
          variant="link"
          className="h-auto p-0 font-medium"
        >
          {display(client.hostname)}
        </Button>
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {display(client.agent_id)}
      </TableCell>
      <TableCell className="font-mono text-xs">
        {formatIP(display(client.client_ip))}
      </TableCell>
      <TableCell>
        <OSLabel value={client.os_version} />
      </TableCell>
      <TableCell>{formatPercent(client.cpu_load)}</TableCell>
      <TableCell>{formatPercent(client.ram_usage)}</TableCell>
      <TableCell>
        <div>{formatRelative(client.last_seen)}</div>
        <div className="text-xs text-muted-foreground">
          {formatDate(client.last_seen)}
        </div>
      </TableCell>
      <TableCell className="text-right">
        <Button
          render={
            <Link to="/agents/$agentId" params={{ agentId: client.agent_id }} />
          }
          nativeButton={false}
          variant="ghost"
          size="icon-sm"
          aria-label={`Open ${display(client.hostname)}`}
        >
          <ChevronRight data-icon="inline-start" />
        </Button>
      </TableCell>
    </TableRow>
  )
}
