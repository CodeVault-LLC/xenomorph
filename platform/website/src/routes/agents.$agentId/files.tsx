import { createFileRoute } from "@tanstack/react-router"

import { FileExplorer } from "@/components/dashboard/file-explorer"
import { useAgentClient } from "@/components/data/clients-query"

export const Route = createFileRoute("/agents/$agentId/files")({
  component: FilesTab,
})

function FilesTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)

  if (!client) {
    return null
  }

  return <FileExplorer agentID={client.agent_id} />
}
