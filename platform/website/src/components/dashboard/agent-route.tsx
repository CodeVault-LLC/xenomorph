import { getRouteApi } from "@tanstack/react-router"

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

const agentRouteApi = getRouteApi("/agents/$agentId")

export function AgentRoute() {
  const { agentId } = agentRouteApi.useParams()
  const { clients, error, isPending, refresh } = useClients()
  const client = clients.find((item) => item.agent_id === agentId)

  return (
    <PageShell className="max-w-none">
      <section className="flex min-w-0 flex-col gap-5">
        <ErrorBanner
          message={error}
          onRetry={refresh}
          requestPath="/api/clients"
        />

        {client ? (
          <AgentWorkspace client={client} />
        ) : (
          <Card>
            <CardContent>
              {isPending ? (
                <div className="flex flex-col gap-3" aria-busy="true">
                  <Skeleton className="h-5 w-36" />
                  <Skeleton className="h-4 w-full max-w-lg" />
                </div>
              ) : error ? (
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <span className="font-mono text-xs">!</span>
                    </EmptyMedia>
                    <EmptyTitle>Agent data is unavailable</EmptyTitle>
                    <EmptyDescription>
                      Restore the gateway connection, then try again. The
                      dashboard has not changed the agent.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
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
