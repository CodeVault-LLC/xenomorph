import { createFileRoute } from "@tanstack/react-router"

import { AgentWorkspace } from "@/components/dashboard/agent-workspace"
import { useClients } from "@/components/data/use-clients"
import { ErrorBanner } from "@/components/layout/error-banner"
import { PageShell } from "@/components/layout/page-shell"
import { Card, CardContent } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"

export const Route = createFileRoute("/agents/$agentId")({
  component: AgentRoute,
})

function AgentRoute() {
  const { agentId } = Route.useParams()
  const { clients, loading, error } = useClients()
  const client = clients.find((item) => item.agent_id === agentId)

  return (
    <PageShell className="max-w-none">
      <section className="flex min-w-0 flex-col gap-5">
        <ErrorBanner message={error} />

        {client ? (
          <AgentWorkspace client={client} />
        ) : (
          <Card>
            <CardContent>
              {loading ? (
                <div className="flex flex-col gap-3">
                  <Skeleton className="h-5 w-36" />
                  <Skeleton className="h-4 w-full max-w-lg" />
                </div>
              ) : (
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <span className="font-mono text-xs">404</span>
                    </EmptyMedia>
                    <EmptyTitle>Agent not found</EmptyTitle>
                    <EmptyDescription>
                      {agentId} has not been observed during the current gateway
                      process lifetime.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              )}
            </CardContent>
          </Card>
        )}
      </section>
    </PageShell>
  )
}
