import { createFileRoute } from "@tanstack/react-router"

import { AgentWorkspace } from "@/components/dashboard/agent-workspace"
import { useClients } from "@/components/data/use-clients"
import { ErrorBanner } from "@/components/layout/error-banner"
import { PageHeader } from "@/components/layout/page-header"
import { PageShell } from "@/components/layout/page-shell"
import { RefreshControl } from "@/components/layout/refresh-control"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { formatDate } from "@/lib/clients"

export const Route = createFileRoute("/agents/$agentId")({
  component: AgentRoute,
})

function AgentRoute() {
  const { agentId } = Route.useParams()
  const { clients, loading, error, updatedAt, refresh } = useClients()
  const client = clients.find((item) => item.agent_id === agentId)

  return (
    <PageShell className="max-w-none">
      <section className="flex min-w-0 flex-col gap-5">
        <PageHeader
          title="Workspace"
          description={<span className="font-mono text-xs">{agentId}</span>}
          actions={
            <RefreshControl
              updatedAt={updatedAt}
              loading={loading}
              onRefresh={refresh}
              format={formatDate}
            />
          }
        />

        <ErrorBanner message={error} />

        {client ? (
          <AgentWorkspace client={client} />
        ) : (
          <Card>
            <CardHeader>
              <CardTitle>
                {loading ? "Loading agent" : "Agent not found"}
              </CardTitle>
              <CardDescription>{agentId}</CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              {loading
                ? "Reading the gateway client directory."
                : "This agent has not been observed during the current gateway process lifetime."}
            </CardContent>
          </Card>
        )}
      </section>
    </PageShell>
  )
}
