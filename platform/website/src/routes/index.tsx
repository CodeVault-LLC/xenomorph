import { createFileRoute } from "@tanstack/react-router"

import { ClientTable } from "@/components/dashboard/client/client-table"
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
  const { clients, error, isFetching, isPending, updatedAt, refresh } =
    useClients()

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
            loading={isFetching}
            onRefresh={refresh}
            format={formatDate}
          />
        }
      />

      <ErrorBanner message={error} onRetry={refresh} />

      <ClientTable clients={clients} loading={isPending} />
    </PageShell>
  )
}
