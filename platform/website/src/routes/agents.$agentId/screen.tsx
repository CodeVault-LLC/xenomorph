import { createFileRoute } from "@tanstack/react-router"

import { LiveScreen } from "@/components/dashboard/live-screen"
import { useAgentClient } from "@/components/data/clients-query"

export const Route = createFileRoute("/agents/$agentId/screen")({
  component: ScreenTab,
})

function ScreenTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)

  if (!client) {
    return null
  }

  return <LiveScreen client={client} />
}
