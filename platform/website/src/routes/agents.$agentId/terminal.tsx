import { createFileRoute } from "@tanstack/react-router"

import { AgentTerminal } from "@/components/dashboard/agent-terminal"
import { useAgentClient } from "@/components/data/clients-query"

export const Route = createFileRoute("/agents/$agentId/terminal")({
  component: TerminalTab,
})

function TerminalTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)

  if (!client) {
    return null
  }

  return <AgentTerminal client={client} />
}
