import * as React from "react"
import {
  Activity,
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
import { OSLabel } from "@/components/dashboard/os-badge"
import { ResourceMeter } from "@/components/dashboard/resource-meter"
import { TermLink } from "@/components/glossary/term-link"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  type ClientSnapshot,
  display,
  formatBytes,
  formatDate,
  formatDuration,
  formatIP,
  formatPercent,
  formatRelative,
  resourceTone,
} from "@/lib/clients"
import { StatusBadge } from "./status-badge"

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
    <Tabs
      value={activeTab}
      onValueChange={(value) => setActiveTab(value as TabID)}
      orientation="vertical"
      className="grid overflow-hidden rounded-lg border border-border bg-card md:h-[calc(100vh-190px)] md:grid-cols-[256px_minmax(0,1fr)]"
    >
      <WorkspaceSidebar client={client} activeTab={activeTab} />

      <div className="min-w-0 overflow-y-auto bg-background p-4 sm:p-5 lg:p-6">
        <TabsContent value="general">
          <GeneralTab client={client} />
        </TabsContent>
        <TabsContent value="system">
          <SystemTab client={client} />
        </TabsContent>
        <TabsContent value="files">
          <FilesTab client={client} />
        </TabsContent>
        <TabsContent value="terminal">
          <AgentTerminal client={client} />
        </TabsContent>
        <TabsContent value="screen">
          <LiveScreen client={client} />
        </TabsContent>
        <TabsContent value="logs">
          <LogsTab client={client} logState={logState} />
        </TabsContent>
      </div>
    </Tabs>
  )
}

function WorkspaceSidebar({
  client,
  activeTab,
}: {
  client: ClientSnapshot
  activeTab: TabID
}) {
  return (
    <Sidebar>
      <SidebarHeader>
        <div className="flex flex-wrap gap-2">
          <OSLabel value={client.os_version} />
        </div>
      </SidebarHeader>

      <SidebarContent className="overflow-hidden">
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <TabsList variant="line" className="h-auto w-full items-stretch p-0">
            <SidebarMenu>
              {tabs.map((tab) => {
                const Icon = tab.icon
                return (
                  <SidebarMenuItem key={tab.id}>
                    <TabsTrigger
                      value={tab.id}
                      className="h-9 w-full justify-start px-2.5"
                      data-active={activeTab === tab.id}
                    >
                      <Icon data-icon="inline-start" />
                      {tab.label}
                    </TabsTrigger>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </TabsList>
        </SidebarGroup>

        <SidebarGroup className="mt-auto pt-4">
          <Separator />

          <div className="grid gap-3 px-2 text-xs">
            <div className="mt-4 flex w-full min-w-0 flex-row items-center justify-between gap-2 overflow-hidden">
              <h2 className="truncate text-lg font-semibold">
                {display(client.hostname)}
              </h2>

              <StatusBadge online={client.is_online} />
            </div>
            <span className="truncate text-muted-foreground">
              {formatIP(client.client_ip)}
            </span>
          </div>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
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

function SystemTab({ client }: { client: ClientSnapshot }) {
  const gpuDevices = Array.isArray(client.gpu_devices) ? client.gpu_devices : []
  const addresses = Array.isArray(client.network_addresses)
    ? client.network_addresses
    : []
  const networkReported = client.network_name.trim() !== ""
  const networkStatus = !networkReported
    ? "Not reported"
    : client.network_online
      ? "Online"
      : "No carrier reported"

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
            label="Current CPU Frequency"
            value={
              client.cpu_frequency_mhz > 0
                ? `${client.cpu_frequency_mhz.toLocaleString()} MHz`
                : "n/a"
            }
          />
          <Fact
            label="CPU Topology"
            value={`${client.cpu_cores || 0} cores / ${client.cpu_threads || 0} threads`}
          />
          <Fact label="Total RAM" value={formatBytes(client.total_ram_bytes)} />
          <Fact
            label="Root Storage"
            value={formatBytes(client.total_storage_bytes)}
          />
          <Fact
            label="Root Storage Available"
            value={formatBytes(client.available_storage_bytes)}
          />
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
              label="Default Interface"
              value={display(client.network_name)}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <Fact label="Medium" value={display(client.network_type)} />
              <Fact
                label="Wireless Network (SSID)"
                value={
                  client.network_type === "wireless"
                    ? display(client.network_ssid)
                    : "Not a wireless interface"
                }
              />
              <Fact label="Link State" value={networkStatus} />
              <Fact
                label="Reported Link Speed"
                value={
                  client.network_link_speed_mbps > 0
                    ? `${client.network_link_speed_mbps.toLocaleString()} Mbps`
                    : "n/a"
                }
              />
              <Fact
                label="IP Addresses"
                value={
                  addresses.length
                    ? `${addresses.length} reported`
                    : "None reported"
                }
              />
            </div>
            {addresses.length ? (
              <div className="grid gap-2">
                {addresses.map((address) => (
                  <Badge
                    key={address}
                    variant="secondary"
                    className="w-fit break-all"
                  >
                    {address}
                  </Badge>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                No interface addresses reported.
              </p>
            )}
            <p className="text-sm text-muted-foreground">
              Wireless credentials are intentionally not collected or displayed.
            </p>
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
          <Button type="button" variant="outline" onClick={logState.refresh}>
            Refresh
          </Button>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {logState.error ? (
          <Alert variant="destructive">
            <AlertDescription>{logState.error}</AlertDescription>
          </Alert>
        ) : null}

        {logState.loading ? (
          <p className="text-sm text-muted-foreground">Reading recent logs.</p>
        ) : null}

        {!logState.loading && logState.logs.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <ScrollText />
              </EmptyMedia>
              <EmptyTitle>No logs recorded</EmptyTitle>
              <EmptyDescription>
                No logs are recorded for {client.agent_id} in the current
                gateway process lifetime.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : null}

        <ScrollArea className="max-h-140">
          <div className="flex flex-col gap-3">
            {logState.logs.map((entry) => (
              <Card key={entry.event_id}>
                <CardHeader>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={logVariant(entry.level)}>
                      {entry.level}
                    </Badge>
                    <span className="font-mono text-xs text-muted-foreground">
                      {display(entry.component)}
                    </span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      {formatDate(entry.observed_at)}
                    </span>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-sm wrap-break-word">
                    {entry.message || "n/a"}
                  </p>
                  <div className="mt-2 font-mono text-xs break-all text-muted-foreground">
                    {entry.event_id}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </ScrollArea>
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
