import * as React from "react"

import { type AgentLogEntry, fetchAgentLogs } from "@/lib/clients"

export type AgentLogsState = {
  logs: AgentLogEntry[]
  loading: boolean
  error: string | null
  refresh: () => void
}

export function useAgentLogs(agentId: string): AgentLogsState {
  const [logs, setLogs] = React.useState<AgentLogEntry[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)

  const refresh = React.useCallback(() => {
    setLoading(true)
    setError(null)
    fetchAgentLogs(agentId)
      .then((snapshot) => {
        setLogs(snapshot)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Agent logs unavailable")
      })
      .finally(() => {
        setLoading(false)
      })
  }, [agentId])

  React.useEffect(() => {
    const initial = window.setTimeout(refresh, 0)
    const interval = window.setInterval(refresh, 5000)
    return () => {
      window.clearTimeout(initial)
      window.clearInterval(interval)
    }
  }, [refresh])

  return { logs, loading, error, refresh }
}
