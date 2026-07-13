import { createFileRoute } from "@tanstack/react-router"

import { Fact } from "@/components/dashboard/fact"
import { useAgentClient } from "@/components/data/clients-query"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

export const Route = createFileRoute("/agents/$agentId/files")({
  component: FilesTab,
})

function FilesTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)

  if (!client) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Files</CardTitle>
        <CardDescription>
          File transfer visibility for this authenticated agent.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <Fact label="Agent" value={client.agent_id} mono />
        <Fact
          label="Transfer Store"
          value="No dashboard file store is configured"
        />
        <p className="text-sm text-muted-foreground sm:col-span-2">
          The shared event schema contains file chunk events, but this dashboard
          currently exposes presence, screen, command audit, and diagnostic log
          data only.
        </p>
      </CardContent>
    </Card>
  )
}
