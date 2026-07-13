import { queryOptions, useQuery } from "@tanstack/react-query"

import { type AgentLogEntry, fetchAgentLogs } from "@/lib/clients"

export type AgentLogsState = {
  logs: AgentLogEntry[]
  error: string | null
  isFetching: boolean
  isPending: boolean
  refresh: () => Promise<unknown>
}

export const agentLogsQueryOptions = (agentId: string) =>
  queryOptions({
    queryKey: ["clients", agentId, "logs"] as const,
    queryFn: () => fetchAgentLogs(agentId),
    enabled: agentId.length > 0,
    refetchInterval: 5_000,
  })

export function useAgentLogs(agentId: string): AgentLogsState {
  const query = useQuery(agentLogsQueryOptions(agentId))

  return {
    logs: query.data ?? [],
    error: query.error?.message ?? null,
    isFetching: query.isFetching,
    isPending: query.isPending,
    refresh: query.refetch,
  }
}
