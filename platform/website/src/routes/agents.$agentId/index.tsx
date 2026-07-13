import { createFileRoute } from "@tanstack/react-router"
import {
  Clock,
  Cpu,
  Database,
  MemoryStick,
  Monitor,
  Network,
  ShieldCheck,
  Wifi,
} from "lucide-react"

import { Fact } from "@/components/dashboard/fact"
import { MetricCard } from "@/components/dashboard/metric-card"
import { ResourceMeter } from "@/components/dashboard/resource-meter"
import { TermLink } from "@/components/glossary/term-link"
import { Badge } from "@/components/ui/badge"
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
  formatPercent,
  formatRelative,
  resourceTone,
} from "@/lib/clients"
import { useAgentClient } from "@/components/data/clients-query"

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
  const addresses = Array.isArray(client.network_addresses)
    ? client.network_addresses
    : []

  return (
    <div className="flex flex-col gap-5">
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Wifi}
          label="Presence"
          value={client.is_online ? "Online" : "Offline"}
          tone={client.is_online ? "good" : "danger"}
        />
        <MetricCard
          icon={Clock}
          label="Last Connected"
          value={formatRelative(client.last_seen)}
        />
        <MetricCard
          icon={Cpu}
          label="CPU Load"
          value={formatPercent(client.cpu_load)}
          tone={resourceTone(client.cpu_load)}
        />
        <MetricCard
          icon={MemoryStick}
          label="RAM Usage"
          value={formatPercent(client.ram_usage)}
          tone={resourceTone(client.ram_usage)}
        />
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(360px,0.6fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Analytical Overview</CardTitle>
            <CardDescription>
              Current gateway presence with client-authored resource pressure.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 lg:grid-cols-2">
            <ResourceMeter
              label="CPU Pressure"
              value={client.cpu_load}
              detail={display(client.cpu_model)}
            />
            <ResourceMeter
              label="Memory Pressure"
              value={client.ram_usage}
              detail={`${formatBytes(client.total_ram_bytes)} total reported RAM`}
            />
            <ResourceMeter
              label="Disk Utilization"
              value={client.storage_usage || 0}
              detail={`${formatBytes(client.used_storage_bytes)} used of ${formatBytes(client.total_storage_bytes)}`}
            />
            <Fact
              icon={Monitor}
              label="GPU"
              value={
                gpuDevices.length
                  ? `${gpuDevices.length} reported device${gpuDevices.length === 1 ? "" : "s"}`
                  : "No GPU devices reported"
              }
              className="rounded-lg border p-4"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Relationship</CardTitle>
            <CardDescription>
              Gateway-observed connection state for this process lifetime.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <Fact
              label="First Observed"
              value={formatDate(client.first_seen)}
            />
            <Fact label="Last Heartbeat" value={formatDate(client.last_seen)} />
            <Fact label="Last Online" value={formatDate(client.last_online)} />
            <Fact
              label="Known To Gateway"
              value={formatWindow(client.first_seen, client.last_seen)}
            />
            <Fact
              label="Current Session"
              value={
                client.is_online
                  ? formatDuration(client.uptime_seconds)
                  : "Outside heartbeat window"
              }
            />
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-5 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <CardHeader>
            <CardTitle>Current State</CardTitle>
            <CardDescription>
              Gateway-authored identity and address data alongside reported host
              labels.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <Fact
              label="Status"
              value={client.is_online ? "Online" : "Offline"}
            />
            <Fact label="Hostname" value={display(client.hostname)} />
            <Fact label="Client IP" value={display(client.client_ip)} mono />
            <Fact label="OS" value={display(client.os_version)} />
            <Fact
              label="CPU Topology"
              value={`${client.cpu_cores || 0} cores / ${client.cpu_threads || 0} threads`}
            />
            <Fact label="Kernel" value={display(client.kernel_version)} />
            <Fact label="Network" value={display(client.network_name)} />
            <Fact
              label="Interfaces"
              value={
                addresses.length
                  ? `${addresses.length} address${addresses.length === 1 ? "" : "es"}`
                  : "No addresses reported"
              }
            />
            <Fact
              label="Agent ID"
              value={client.agent_id}
              mono
              className="sm:col-span-2 xl:col-span-1"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="size-4 text-muted-foreground" />
              Trust Source
            </CardTitle>
            <CardDescription>How fields should be interpreted.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm text-muted-foreground">
            <p>
              Agent ID, client IP, timestamps, and online state are{" "}
              <TermLink slug="gateway">gateway</TermLink>-authored.
            </p>
            <p>
              Hostname, OS, hardware, network labels, screen, and logs are{" "}
              <TermLink slug="telemetry">agent-authored</TermLink> and not
              identity evidence.
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Reported Inventory</CardTitle>
          <CardDescription>
            Minimal host inventory received from the agent heartbeat.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-3">
          <Fact
            icon={Database}
            label="Total RAM"
            value={formatBytes(client.total_ram_bytes)}
          />
          <Fact
            icon={Cpu}
            label="CPU Model"
            value={display(client.cpu_model)}
          />
          <Fact
            icon={Network}
            label="Network Addresses"
            value={addresses.length ? "" : "No interface addresses reported"}
          />
          {addresses.length ? (
            <div className="flex flex-wrap gap-2 md:col-span-3">
              {addresses.map((address) => (
                <Badge key={address} variant="secondary" className="break-all">
                  {address}
                </Badge>
              ))}
            </div>
          ) : null}
        </CardContent>
      </Card>
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
