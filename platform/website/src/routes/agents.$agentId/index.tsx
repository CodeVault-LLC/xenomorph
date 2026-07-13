import { createFileRoute } from "@tanstack/react-router"
import {
  Clock,
  Cpu,
  Fingerprint,
  Globe2,
  MemoryStick,
  Wifi,
} from "lucide-react"

import { useAgentClient } from "@/components/data/clients-query"
import { Fact } from "@/components/dashboard/fact"
import { MetricCard } from "@/components/dashboard/metric-card"
import { OSBadge } from "@/components/dashboard/os-badge"
import { ResourceMeter } from "@/components/dashboard/resource-meter"
import { StatusBadge } from "@/components/dashboard/status-badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  type ClientSnapshot,
  display,
  formatBytes,
  formatDate,
  formatDuration,
  formatRelative,
} from "@/lib/clients"

export const Route = createFileRoute("/agents/$agentId/")({
  component: GeneralTab,
})

function GeneralTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)

  if (!client) {
    return null
  }

  return <GeneralContent client={client} />
}

function GeneralContent({ client }: { client: ClientSnapshot }) {
  const gpuDevices = Array.isArray(client.gpu_devices) ? client.gpu_devices : []

  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-col gap-3 border-b border-border pb-5 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge online={client.is_online} />
            <OSBadge value={client.os_version} detailed />
          </div>
          <h1 className="mt-3 truncate text-2xl font-semibold tracking-tight">
            {display(client.hostname)}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Client-reported hostname · gateway record
          </p>
        </div>
        <div className="min-w-0 sm:max-w-xs sm:text-right">
          <div className="text-xs font-medium text-muted-foreground uppercase">
            Gateway agent ID
          </div>
          <div className="mt-1 font-mono text-xs break-all">
            {client.agent_id}
          </div>
        </div>
      </header>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Wifi}
          label="Presence"
          value={client.is_online ? "Online" : "Offline"}
          tone={client.is_online ? "good" : "danger"}
        />
        <MetricCard
          icon={Clock}
          label="Last Heartbeat"
          value={formatRelative(client.last_seen)}
        />
        <MetricCard
          icon={Cpu}
          label="Current Session"
          value={
            client.is_online
              ? formatDuration(client.uptime_seconds)
              : "Not connected"
          }
        />
        <MetricCard
          icon={MemoryStick}
          label="Known To Gateway"
          value={formatWindow(client.first_seen, client.last_seen)}
        />
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.25fr)_minmax(320px,0.75fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Resource pressure</CardTitle>
            <CardDescription>
              Latest utilization reported by the agent. These values describe
              telemetry, not host integrity.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-3">
            <ResourceMeter
              label="CPU"
              value={client.cpu_load}
              detail={display(client.cpu_model)}
            />
            <ResourceMeter
              label="Memory"
              value={client.ram_usage}
              detail={`${formatBytes(client.total_ram_bytes)} total reported RAM`}
            />
            <ResourceMeter
              label="Storage"
              value={client.storage_usage || 0}
              detail={`${formatBytes(client.used_storage_bytes)} used of ${formatBytes(client.total_storage_bytes)}`}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Reported profile</CardTitle>
            <CardDescription>
              A compact client-authored snapshot. Inspect full hardware and
              network data in System.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
            <Fact label="Operating system" value={display(client.os_version)} />
            <Fact
              label="CPU topology"
              value={`${client.cpu_cores || 0} cores · ${client.cpu_threads || 0} threads`}
            />
            <Fact
              label="GPU"
              value={
                gpuDevices.length
                  ? `${gpuDevices.length} reported device${gpuDevices.length === 1 ? "" : "s"}`
                  : "No devices reported"
              }
            />
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.6fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Gateway observation</CardTitle>
            <CardDescription>
              Identity, address, and presence are recorded by the gateway and
              can distinguish this agent record.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-2">
            <Fact
              label="Status"
              value={client.is_online ? "Online" : "Offline"}
            />
            <Fact
              icon={Globe2}
              label="Client IP"
              value={display(client.client_ip)}
              mono
            />
            <Fact
              icon={Fingerprint}
              label="Agent ID"
              value={client.agent_id}
              mono
            />
            <Fact
              label="First observed"
              value={formatDate(client.first_seen)}
            />
            <Fact label="Last heartbeat" value={formatDate(client.last_seen)} />
            <Fact label="Last online" value={formatDate(client.last_online)} />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function formatWindow(startValue: string, endValue: string) {
  const start = new Date(startValue)
  const end = new Date(endValue)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
    return "Unknown"
  }

  const seconds = Math.max(
    0,
    Math.floor((end.getTime() - start.getTime()) / 1000)
  )
  return formatDuration(seconds)
}
