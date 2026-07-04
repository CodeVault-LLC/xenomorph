import * as React from "react"
import { Link } from "@tanstack/react-router"
import {
  Activity,
  ArrowLeft,
  Clock,
  Cpu,
  Database,
  FileStack,
  HardDrive,
  MemoryStick,
  Monitor,
  Network,
  ScrollText,
  ShieldCheck,
  Terminal,
  Wifi,
} from "lucide-react"

import { useAgentLogs } from "@/components/data/use-agent-logs"
import { AgentTerminal } from "@/components/dashboard/agent-terminal"
import { Fact } from "@/components/dashboard/fact"
import { LiveScreen } from "@/components/dashboard/live-screen"
import { MetricCard } from "@/components/dashboard/metric-card"
import { OSBadge } from "@/components/dashboard/os-badge"
import { ResourceMeter } from "@/components/dashboard/resource-meter"
import { StatusBadge } from "@/components/dashboard/status-badge"
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
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
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

const tabs = [
  { id: "general", label: "General", icon: Activity },
  { id: "system", label: "System", icon: Cpu },
  { id: "files", label: "Files", icon: FileStack },
  { id: "terminal", label: "Terminal", icon: Terminal },
  { id: "screen", label: "Screen", icon: Monitor },
  { id: "logs", label: "Logs", icon: ScrollText },
] as const

type TabID = (typeof tabs)[number]["id"]

export function AgentWorkspace({ client }: { client: ClientSnapshot }) {
  const [activeTab, setActiveTab] = React.useState<TabID>("general")
  const logState = useAgentLogs(client.agent_id)

  return (
    <div className="grid overflow-hidden rounded-lg border border-border bg-card md:min-h-[calc(100vh-190px)] md:grid-cols-[256px_minmax(0,1fr)]">
      <WorkspaceSidebar
        client={client}
        activeTab={activeTab}
        onTabChange={setActiveTab}
      />

      <div className="min-w-0 bg-background p-4 sm:p-5 lg:p-6">
        {activeTab === "general" ? <GeneralTab client={client} /> : null}
        {activeTab === "system" ? <SystemTab client={client} /> : null}
        {activeTab === "files" ? <FilesTab client={client} /> : null}
        {activeTab === "terminal" ? <AgentTerminal client={client} /> : null}
        {activeTab === "screen" ? <LiveScreen client={client} /> : null}
        {activeTab === "logs" ? (
          <LogsTab client={client} logState={logState} />
        ) : null}
      </div>
    </div>
  )
}

function WorkspaceSidebar({
  client,
  activeTab,
  onTabChange,
}: {
  client: ClientSnapshot
  activeTab: TabID
  onTabChange: (tab: TabID) => void
}) {
  return (
    <Sidebar>
      <SidebarHeader>
        <Link
          to="/"
          className="mb-4 inline-flex h-8 w-fit items-center gap-1.5 rounded-md border border-sidebar-border bg-background px-2.5 text-sm font-medium transition-colors hover:bg-sidebar-accent focus-visible:ring-3 focus-visible:ring-sidebar-ring/50"
        >
          <ArrowLeft className="size-4" />
          Clients
        </Link>
        <div className="flex flex-wrap gap-2">
          <StatusBadge online={client.is_online} />
          <OSBadge value={client.os_version} detailed />
        </div>
        <div className="mt-4 min-w-0">
          <h2 className="truncate text-lg font-semibold">
            {display(client.hostname)}
          </h2>
          <p className="mt-1 font-mono text-xs break-all text-sidebar-foreground/60">
            {client.agent_id}
          </p>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <SidebarMenu>
            {tabs.map((tab) => {
              const Icon = tab.icon
              return (
                <SidebarMenuItem key={tab.id}>
                  <SidebarMenuButton
                    type="button"
                    active={activeTab === tab.id}
                    onClick={() => onTabChange(tab.id)}
                  >
                    <Icon />
                    {tab.label}
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )
            })}
          </SidebarMenu>
        </SidebarGroup>

        <SidebarGroup className="mt-auto border-t border-sidebar-border pt-4">
          <SidebarGroupLabel>Gateway View</SidebarGroupLabel>
          <div className="grid gap-3 px-2 text-xs">
            <SidebarStat label="Client IP" value={display(client.client_ip)} />
            <SidebarStat
              label="Last seen"
              value={formatRelative(client.last_seen)}
            />
            <SidebarStat
              label="Session"
              value={
                client.is_online
                  ? formatDuration(client.uptime_seconds)
                  : "Not connected"
              }
            />
          </div>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}

function SidebarStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1">
      <div className="font-medium text-sidebar-foreground/55 uppercase">
        {label}
      </div>
      <div className="break-words text-sidebar-foreground">{value}</div>
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

function GeneralTab({ client }: { client: ClientSnapshot }) {
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
            <Fact
              icon={HardDrive}
              label="Disk Space"
              value="Not reported by current heartbeat"
              className="rounded-lg border p-4"
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
                <code
                  key={address}
                  className="rounded-md border bg-muted px-2 py-1 text-xs break-all"
                >
                  {address}
                </code>
              ))}
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}

function SystemTab({ client }: { client: ClientSnapshot }) {
  const gpuDevices = Array.isArray(client.gpu_devices) ? client.gpu_devices : []
  const addresses = Array.isArray(client.network_addresses)
    ? client.network_addresses
    : []

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Compute</CardTitle>
          <CardDescription>
            Client-authored CPU, GPU, memory, and OS telemetry.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <Fact
            icon={Cpu}
            label="CPU Load"
            value={formatPercent(client.cpu_load)}
          />
          <Fact
            icon={MemoryStick}
            label="RAM Usage"
            value={formatPercent(client.ram_usage)}
          />
          <Fact label="CPU Model" value={display(client.cpu_model)} />
          <Fact
            label="CPU Topology"
            value={`${client.cpu_cores || 0} cores / ${client.cpu_threads || 0} threads`}
          />
          <Fact label="Total RAM" value={formatBytes(client.total_ram_bytes)} />
          <Fact label="Uptime" value={formatDuration(client.uptime_seconds)} />
          <Fact label="OS" value={display(client.os_version)} />
          <Fact label="Kernel" value={display(client.kernel_version)} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Graphics And Network</CardTitle>
          <CardDescription>
            Hardware and connectivity labels reported by the client.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="grid gap-3">
            <Fact
              icon={HardDrive}
              label="GPU Devices"
              value={gpuDevices.length ? "" : "n/a"}
            />
            {gpuDevices.length ? (
              <div className="flex flex-wrap gap-2">
                {gpuDevices.map((gpu) => (
                  <Badge key={gpu} variant="secondary">
                    {gpu}
                  </Badge>
                ))}
              </div>
            ) : null}
          </div>

          <div className="grid gap-3">
            <Fact
              icon={Network}
              label="Connected Network"
              value={display(client.network_name)}
            />
            {addresses.length ? (
              <div className="grid gap-2">
                {addresses.map((address) => (
                  <code
                    key={address}
                    className="rounded-md border bg-muted px-2 py-1 text-xs break-all"
                  >
                    {address}
                  </code>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                No interface addresses reported.
              </p>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function FilesTab({ client }: { client: ClientSnapshot }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Files</CardTitle>
        <CardDescription>
          File transfer visibility for this authenticated agent.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <Fact label="Agent" value={client.agent_id} mono />
        <Fact
          label="Transfer Store"
          value="No dashboard file store is configured"
        />
        <p className="text-sm text-muted-foreground sm:col-span-2">
          The shared event schema contains file chunk events, but this dashboard
          currently exposes presence, screen, command audit, and diagnostic log
          data only.
        </p>
      </CardContent>
    </Card>
  )
}

function LogsTab({
  client,
  logState,
}: {
  client: ClientSnapshot
  logState: ReturnType<typeof useAgentLogs>
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>Logs</CardTitle>
            <CardDescription>
              Recent client diagnostics and gateway command audit entries.
            </CardDescription>
          </div>
          <button
            type="button"
            className="h-9 rounded-md border px-3 text-sm font-medium hover:bg-accent"
            onClick={logState.refresh}
          >
            Refresh
          </button>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {logState.error ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">
            {logState.error}
          </div>
        ) : null}

        {logState.loading ? (
          <p className="text-sm text-muted-foreground">Reading recent logs.</p>
        ) : null}

        {!logState.loading && logState.logs.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No logs are recorded for {client.agent_id} in the current gateway
            process lifetime.
          </p>
        ) : null}

        {logState.logs.map((entry) => (
          <div key={entry.event_id} className="rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={logVariant(entry.level)}>{entry.level}</Badge>
              <span className="font-mono text-xs text-muted-foreground">
                {display(entry.component)}
              </span>
              <span className="ml-auto text-xs text-muted-foreground">
                {formatDate(entry.observed_at)}
              </span>
            </div>
            <p className="mt-2 text-sm break-words">{entry.message || "n/a"}</p>
            <div className="mt-2 font-mono text-xs break-all text-muted-foreground">
              {entry.event_id}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function logVariant(
  level: string
): "secondary" | "offline" | "warning" | "outline" {
  switch (level) {
    case "ERROR":
      return "offline"
    case "WARN":
      return "warning"
    case "DEBUG":
      return "outline"
    default:
      return "secondary"
  }
}
