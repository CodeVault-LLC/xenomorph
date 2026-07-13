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
  uptime_seconds: number
  cpu_model: string
  cpu_cores: number
  cpu_threads: number
  total_ram_bytes: number
  gpu_devices: string[]
  network_name: string
  network_addresses: string[]
  kernel_version: string
  cpu_frequency_mhz: number
  network_online: boolean
  network_link_speed_mbps: number
  network_type: string
  total_storage_bytes: number
  available_storage_bytes: number
  used_storage_bytes: number
  storage_usage: number
  storage_inode_usage: number
  storage_device: string
  storage_filesystem: string
  storage_mountpoint: string
  storage_model: string
  storage_type: string
  storage_read_only: boolean
  application_types: ApplicationTypeUsage[]
  network_ssid: string
  first_seen: string
  last_seen: string
  last_online: string
  is_online: boolean
}

export type ApplicationTypeUsage = {
  category: string
  count: number
}

export type AgentLogEntry = {
  event_id: string
  agent_id: string
  client_ip: string
  observed_at: string
  level: "DEBUG" | "INFO" | "WARN" | "ERROR" | string
  component: string
  message: string
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

type AgentLogsResponse = {
  logs?: AgentLogEntry[]
}

export async function fetchClients() {
  const response = await fetch("/api/clients", { cache: "no-store" })
  if (!response.ok) {
    throw new Error(`Client API returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as ClientsResponse
  return Array.isArray(payload.clients) ? payload.clients : []
}

export async function fetchAgentLogs(agentId: string) {
  const response = await fetch(`/api/clients/${agentId}/logs`, {
    cache: "no-store",
  })
  if (!response.ok) {
    throw new Error(`Agent log API returned HTTP ${response.status}`)
  }

  const payload = (await response.json()) as AgentLogsResponse
  return Array.isArray(payload.logs) ? payload.logs : []
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

  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000))
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

export function formatBytes(value: number) {
  if (typeof value !== "number" || Number.isNaN(value) || value <= 0) {
    return "n/a"
  }

  const units = ["B", "KiB", "MiB", "GiB", "TiB"]
  let size = value
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }

  return `${size.toFixed(size >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

export function formatDuration(seconds: number) {
  if (typeof seconds !== "number" || Number.isNaN(seconds) || seconds <= 0) {
    return "n/a"
  }

  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  if (days > 0) {
    return `${days}d ${hours}h`
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }
  return `${minutes}m`
}

export function display(value: string) {
  return typeof value !== "string" || value.trim() === "" ? "n/a" : value
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

export const formatIP = (value: string) => {
  if (value === "::1") {
    return "localhost"
  }

  if (value.startsWith("::ffff:")) {
    return value.replace("::ffff:", "")
  }

  return value
}
