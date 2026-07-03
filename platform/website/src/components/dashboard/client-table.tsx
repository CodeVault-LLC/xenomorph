import { Link } from "@tanstack/react-router"
import { ChevronRight } from "lucide-react"

import { OSLabel } from "@/components/dashboard/os-badge"
import { StatusBadge } from "@/components/dashboard/status-badge"
import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  type ClientSnapshot,
  display,
  formatDate,
  formatPercent,
  formatRelative,
} from "@/lib/clients"
import { cn } from "@/lib/utils"

const columns = [
  "Status",
  "Hostname",
  "Agent ID",
  "IP",
  "OS",
  "CPU",
  "RAM",
  "Last Seen",
  "",
]

export function ClientTable({
  clients,
  loading,
}: {
  clients: ClientSnapshot[]
  loading: boolean
}) {
  return (
    <Card className="overflow-hidden">
      <div className="overflow-x-auto">
        <Table className="min-w-[1080px]">
          <TableHeader>
            <TableRow>
              {columns.map((column) => (
                <TableHead key={column}>{column}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {clients.length > 0 ? (
              clients.map((client) => (
                <ClientRow key={client.agent_id} client={client} />
              ))
            ) : (
              <TableRow>
                <TableCell
                  className="py-10 text-center text-muted-foreground"
                  colSpan={columns.length}
                >
                  {loading
                    ? "Loading clients..."
                    : "No clients have connected during this gateway process lifetime."}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </Card>
  )
}

function ClientRow({ client }: { client: ClientSnapshot }) {
  return (
    <TableRow className="group hover:bg-muted/50">
      <TableCell>
        <StatusBadge online={client.is_online} />
      </TableCell>
      <TableCell className="font-medium">
        <Link
          to="/agents/$agentId"
          params={{ agentId: client.agent_id }}
          className="outline-none hover:underline focus-visible:underline"
        >
          {display(client.hostname)}
        </Link>
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {display(client.agent_id)}
      </TableCell>
      <TableCell className="font-mono text-xs">
        {display(client.client_ip)}
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
        <Link
          to="/agents/$agentId"
          params={{ agentId: client.agent_id }}
          className={cn(
            "inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-sm font-medium transition-all outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
          )}
        >
          <ChevronRight />
        </Link>
      </TableCell>
    </TableRow>
  )
}
