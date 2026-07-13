import { queryOptions, useQuery, useQueryClient } from "@tanstack/react-query"
import * as React from "react"

import {
  type ClientSnapshot,
  fetchClients,
  subscribeClients,
} from "@/lib/clients"

const clientsQueryKey = ["clients"] as const
const CLIENTS_STALE_TIME_MS = 5_000

export const clientsQueryOptions = queryOptions({
  queryKey: clientsQueryKey,
  queryFn: fetchClients,
  staleTime: CLIENTS_STALE_TIME_MS,
})

export type ClientsQueryState = {
  clients: ClientSnapshot[]
  error: string | null
  isFetching: boolean
  isPending: boolean
  updatedAt: Date | null
  refresh: () => Promise<unknown>
}

export function useClientsQuery(): ClientsQueryState {
  const query = useQuery(clientsQueryOptions)

  return {
    clients: query.data ?? [],
    error: query.error?.message ?? null,
    isFetching: query.isFetching,
    isPending: query.isPending,
    updatedAt: query.dataUpdatedAt ? new Date(query.dataUpdatedAt) : null,
    refresh: query.refetch,
  }
}

export function useAgentClient(agentId: string) {
  const { clients } = useClientsQuery()

  return clients.find((client) => client.agent_id === agentId)
}

/** Keeps the clients query cache in sync with gateway-issued snapshots. */
export function ClientsStream() {
  const queryClient = useQueryClient()

  React.useEffect(() => {
    return subscribeClients((clients) => {
      queryClient.setQueryData<ClientSnapshot[]>(clientsQueryKey, clients)
    })
  }, [queryClient])

  return null
}
