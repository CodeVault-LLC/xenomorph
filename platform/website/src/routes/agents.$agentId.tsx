import { createFileRoute } from "@tanstack/react-router"

import { clientsQueryOptions } from "@/components/data/clients-query"
import { AgentRoute } from "@/components/dashboard/agent-route"

export const Route = createFileRoute("/agents/$agentId")({
  component: AgentRoute,
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(clientsQueryOptions)
  },
})
