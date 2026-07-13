import { createFileRoute } from "@tanstack/react-router"
import { LoaderCircle, RefreshCw, ScrollText } from "lucide-react"

import { useAgentLogs } from "@/components/data/use-agent-logs"
import { useAgentClient } from "@/components/data/clients-query"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Alert, AlertDescription } from "@/components/ui/alert"
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
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>Logs</CardTitle>
            <CardDescription>
              Recent client diagnostics and gateway command audit entries.
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
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {logState.error ? (
          <Alert variant="destructive">
            <AlertDescription>{logState.error}</AlertDescription>
          </Alert>
        ) : null}

        {logState.isPending ? (
          <p className="text-sm text-muted-foreground">Reading recent logs.</p>
        ) : null}

        {!logState.isPending && logState.logs.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <ScrollText />
              </EmptyMedia>
              <EmptyTitle>No logs recorded</EmptyTitle>
              <EmptyDescription>
                No logs are recorded for {client.agent_id} in the current
                gateway process lifetime.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : null}

        <ScrollArea className="max-h-140">
          <div className="flex flex-col gap-3">
            {logState.logs.map((entry) => (
              <Card key={entry.event_id}>
                <CardHeader>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={logVariant(entry.level)}>
                      {entry.level}
                    </Badge>
                    <span className="font-mono text-xs text-muted-foreground">
                      {display(entry.component)}
                    </span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      {formatDate(entry.observed_at)}
                    </span>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-sm wrap-break-word">
                    {entry.message || "n/a"}
                  </p>
                  <div className="mt-2 font-mono text-xs break-all text-muted-foreground">
                    {entry.event_id}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
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
