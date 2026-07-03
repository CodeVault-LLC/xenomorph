import * as React from "react"

import {
  type ClientSnapshot,
  fetchClients,
  subscribeClients,
} from "@/lib/clients"

export type ClientsState = {
  clients: ClientSnapshot[]
  loading: boolean
  error: string | null
  updatedAt: Date | null
  refresh: () => void
}

export function useClients(): ClientsState {
  const [clients, setClients] = React.useState<ClientSnapshot[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)
  const [updatedAt, setUpdatedAt] = React.useState<Date | null>(null)

  const refresh = React.useCallback(() => {
    setError(null)
    fetchClients()
      .then((snapshot) => {
        setClients(snapshot)
        setUpdatedAt(new Date())
      })
      .catch((err: unknown) => {
        setError(
          err instanceof Error ? err.message : "Client status unavailable"
        )
      })
      .finally(() => {
        setLoading(false)
      })
  }, [])

  React.useEffect(() => {
    return subscribeClients(
      (snapshot) => {
        setClients(snapshot)
        setUpdatedAt(new Date())
        setLoading(false)
        setError(null)
      },
      (message) => {
        setError(message)
        setLoading(false)
      }
    )
  }, [])

  return { clients, loading, error, updatedAt, refresh }
}
