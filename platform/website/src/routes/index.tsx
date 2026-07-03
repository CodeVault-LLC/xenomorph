import { createFileRoute } from "@tanstack/react-router"
import { CircleDot, UsersRound } from "lucide-react"

import { ClientTable } from "@/components/dashboard/client-table"
import { MetricCard } from "@/components/dashboard/metric-card"
import { useClients } from "@/components/data/use-clients"
import { ErrorBanner } from "@/components/layout/error-banner"
import { PageHeader } from "@/components/layout/page-header"
import { PageShell } from "@/components/layout/page-shell"
import { RefreshControl } from "@/components/layout/refresh-control"
import { formatDate } from "@/lib/clients"

export const Route = createFileRoute("/")({
  component: ClientsRoute,
})

function ClientsRoute() {
  const { clients, loading, error, updatedAt, refresh } = useClients()

  const onlineCount = clients.filter((client) => client.is_online).length
  const offlineCount = clients.length - onlineCount

  return (
    <PageShell>
      <PageHeader
        title="Clients"
        description={`${clients.length} known · ${onlineCount} online · ${offlineCount} offline`}
        actions={
          <RefreshControl
            updatedAt={updatedAt}
            loading={loading}
            onRefresh={refresh}
            format={formatDate}
          />
        }
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <MetricCard
          icon={UsersRound}
          label="Known"
          value={clients.length.toString()}
        />
        <MetricCard
          icon={CircleDot}
          label="Online"
          value={onlineCount.toString()}
          tone="good"
        />
        <MetricCard
          icon={CircleDot}
          label="Offline"
          value={offlineCount.toString()}
          tone={offlineCount > 0 ? "warn" : "good"}
        />
      </div>

      <ErrorBanner message={error} />

      <ClientTable clients={clients} loading={loading} />
    </PageShell>
  )
}
