import { MonitorCheck } from "lucide-react"

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
import { type ClientSnapshot } from "@/lib/clients"
import { ClientRow } from "./client-row"

export const ClientTable = ({
  clients,
  loading,
}: {
  clients: ClientSnapshot[]
  loading: boolean
}) => {
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
          <Table className="min-w-270">
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
