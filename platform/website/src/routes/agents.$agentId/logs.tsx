import { createFileRoute } from "@tanstack/react-router"
import {
  AlertTriangle,
  LoaderCircle,
  RefreshCw,
  ScrollText,
} from "lucide-react"

import { useAgentClient } from "@/components/data/clients-query"
import { useAgentLogs } from "@/components/data/use-agent-logs"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
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
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { display, formatDate } from "@/lib/clients"

export const Route = createFileRoute("/agents/$agentId/logs")({
  component: LogsTab,
})

function LogsTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)
  const logState = useAgentLogs(agentId)

  if (!client) {
    return null
  }

  return (
    <Card className="flex min-h-[32rem] flex-col overflow-hidden md:h-[calc(100dvh-238px)]">
      <CardHeader className="gap-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex flex-col gap-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle>Activity logs</CardTitle>
              {!logState.isPending ? (
                <Badge variant="secondary">
                  {logState.logs.length} record
                  {logState.logs.length === 1 ? "" : "s"}
                </Badge>
              ) : null}
            </div>
            <CardDescription>
              Recent diagnostics and command audit events for {client.agent_id}.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={logState.refresh}
            disabled={logState.isFetching}
          >
            {logState.isFetching ? (
              <LoaderCircle className="animate-spin" data-icon="inline-start" />
            ) : (
              <RefreshCw data-icon="inline-start" />
            )}
            Refresh
          </Button>
        </div>

        <p className="text-xs text-muted-foreground">
          Event IDs and observed times are assigned by the gateway. Level,
          component, and message are processed event payload fields.
        </p>
      </CardHeader>

      {logState.error ? (
        <div className="px-4 pt-4">
          <Alert variant="destructive">
            <AlertTriangle />
            <AlertTitle>Unable to refresh logs</AlertTitle>
            <AlertDescription>{logState.error}</AlertDescription>
          </Alert>
        </div>
      ) : null}

      <CardContent className="flex min-h-0 flex-1 flex-col p-0">
        {logState.isPending ? <LogsSkeleton /> : null}

        {!logState.isPending && logState.logs.length === 0 ? (
          <div className="flex min-h-0 flex-1 items-center justify-center p-4">
            <Empty className="max-w-md border">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ScrollText />
                </EmptyMedia>
                <EmptyTitle>No activity recorded</EmptyTitle>
                <EmptyDescription>
                  No recent diagnostic or audit events are available for this
                  agent in the current gateway process lifetime.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </div>
        ) : null}

        {!logState.isPending && logState.logs.length > 0 ? (
          <ScrollArea
            className="min-h-0 flex-1"
            aria-label="Agent activity logs"
          >
            <ol className="divide-y divide-border">
              {logState.logs.map((entry) => (
                <li key={entry.event_id} className="px-4 py-3 sm:px-5">
                  <div className="grid gap-2 lg:grid-cols-[11.5rem_7rem_minmax(0,1fr)] lg:gap-x-4">
                    <time
                      className="text-xs text-muted-foreground"
                      dateTime={entry.observed_at}
                    >
                      {formatDate(entry.observed_at)}
                    </time>
                    <div className="flex min-w-0 items-center gap-2">
                      <Badge variant={logVariant(entry.level)}>
                        {entry.level || "INFO"}
                      </Badge>
                      <span className="truncate font-mono text-xs text-muted-foreground lg:hidden">
                        {display(entry.component)}
                      </span>
                    </div>
                    <div className="flex min-w-0 flex-col gap-1">
                      <span className="hidden font-mono text-xs text-muted-foreground lg:block">
                        {display(entry.component)}
                      </span>
                      <p className="text-sm leading-5 wrap-break-word">
                        {entry.message || "No message provided."}
                      </p>
                      <span
                        title={entry.event_id}
                        className="truncate font-mono text-xs text-muted-foreground"
                      >
                        Event {entry.event_id || "unavailable"}
                      </span>
                    </div>
                  </div>
                </li>
              ))}
            </ol>
          </ScrollArea>
        ) : null}
      </CardContent>
    </Card>
  )
}

function LogsSkeleton() {
  return (
    <div className="flex flex-col gap-4 p-4 sm:p-5">
      {Array.from({ length: 6 }, (_, index) => (
        <div
          key={index}
          className="grid gap-2 border-b border-border pb-4 last:border-b-0 lg:grid-cols-[11.5rem_7rem_minmax(0,1fr)] lg:gap-x-4"
        >
          <Skeleton className="h-4 w-36" />
          <Skeleton className="h-6 w-16 rounded-full" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </div>
        </div>
      ))}
    </div>
  )
}

function logVariant(
  level: string
): "secondary" | "offline" | "warning" | "outline" {
  switch (level) {
    case "ERROR":
      return "offline"
    case "WARN":
      return "warning"
    case "DEBUG":
      return "outline"
    default:
      return "secondary"
  }
}
