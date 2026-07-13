import { Link } from "@tanstack/react-router"
import { MonitorCheck, ChevronRight } from "lucide-react"

import { OSLabel } from "@/components/dashboard/os-badge"
import { StatusBadge } from "@/components/dashboard/status-badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
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
      <CardHeader>
        <CardTitle>Connected Clients</CardTitle>
        <CardDescription>
          Gateway-observed client sessions for this process lifetime.
        </CardDescription>
      </CardHeader>
      <CardContent className="p-0">
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
              ) : loading ? (
                <ClientTableSkeleton />
              ) : (
                <TableRow>
                  <TableCell className="py-10" colSpan={columns.length}>
                    <Empty>
                      <EmptyHeader>
                        <EmptyMedia variant="icon">
                          <MonitorCheck />
                        </EmptyMedia>
                        <EmptyTitle>No clients connected</EmptyTitle>
                        <EmptyDescription>
                          No clients have connected during this gateway process
                          lifetime.
                        </EmptyDescription>
                      </EmptyHeader>
                    </Empty>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}

function ClientTableSkeleton() {
  return Array.from({ length: 5 }).map((_, index) => (
    <TableRow key={index}>
      {columns.map((column) => (
        <TableCell key={column || "action"}>
          <Skeleton className="h-5 w-full max-w-28" />
        </TableCell>
      ))}
    </TableRow>
  ))
}

function ClientRow({ client }: { client: ClientSnapshot }) {
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
