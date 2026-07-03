import {
  Laptop,
  Monitor,
  Server,
  Terminal,
  type LucideIcon,
} from "lucide-react"

export type ClientSnapshot = {
  agent_id: string
  hostname: string
  client_ip: string
  os_version: string
  cpu_load: number
  ram_usage: number
  first_seen: string
  last_seen: string
  last_online: string
  is_online: boolean
}

export type OSInfo = {
  family: string
  label: string
  icon: LucideIcon
  variant: "arch" | "linux" | "windows" | "macos" | "unknown"
}

type ClientsResponse = {
  clients?: ClientSnapshot[]
}

export async function fetchClients() {
  const response = await fetch("/api/clients", { cache: "no-store" })
  if (!response.ok) {
    throw new Error(`Client API returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as ClientsResponse
  return Array.isArray(payload.clients) ? payload.clients : []
}

export function subscribeClients(
  onSnapshot: (clients: ClientSnapshot[]) => void,
  onError: (message: string) => void
) {
  const events = new EventSource("/api/clients/stream")

  events.addEventListener("snapshot", (event) => {
    try {
      const payload = JSON.parse(event.data) as ClientsResponse
      onSnapshot(Array.isArray(payload.clients) ? payload.clients : [])
    } catch {
      onError("Client stream returned invalid data")
    }
  })

  events.onerror = () => {
    onError("Client stream disconnected")
  }

  return () => events.close()
}

export function formatDate(value: string | Date) {
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) {
    return "Unknown"
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(date)
}

export function formatRelative(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return "Unknown"
  }

  const seconds = Math.max(
    0,
    Math.floor((Date.now() - date.getTime()) / 1000)
  )
  if (seconds < 60) {
    return `${seconds}s ago`
  }

  const minutes = Math.round(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ago`
  }

  const hours = Math.round(minutes / 60)
  if (hours < 48) {
    return `${hours}h ago`
  }

  return formatDate(date)
}

export function formatPercent(value: number) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "n/a"
  }

  return `${Math.round(value * 100)}%`
}

export function display(value: string) {
  return value.trim() === "" ? "n/a" : value
}

export function detectOS(value: string) {
  const normalized = value.toLowerCase()
  const label = value.split("/")[0]?.trim() || "Unknown"
  if (normalized.includes("windows")) {
    return {
      family: "Windows",
      label,
      icon: Monitor,
      variant: "windows",
    } satisfies OSInfo
  }
  if (normalized.includes("darwin") || normalized.includes("mac")) {
    return {
      family: "macOS",
      label,
      icon: Laptop,
      variant: "macos",
    } satisfies OSInfo
  }
  if (normalized.includes("arch") || normalized.includes("omarchy")) {
    return {
      family: "Arch",
      label,
      icon: Terminal,
      variant: "arch",
    } satisfies OSInfo
  }
  if (normalized.includes("linux")) {
    return {
      family: "Linux",
      label,
      icon: Terminal,
      variant: "linux",
    } satisfies OSInfo
  }
  if (normalized.includes("bsd")) {
    return {
      family: "BSD",
      label,
      icon: Server,
      variant: "unknown",
    } satisfies OSInfo
  }
  return {
    family: "Unknown",
    label,
    icon: Server,
    variant: "unknown",
  } satisfies OSInfo
}

export function resourceTone(value: number) {
  if (value >= 0.85) {
    return "danger"
  }
  if (value >= 0.7) {
    return "warn"
  }
  return "good"
}
