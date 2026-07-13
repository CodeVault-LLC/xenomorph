import { createFileRoute } from "@tanstack/react-router"
import { Cpu, HardDrive, MemoryStick, Network } from "lucide-react"

import { Fact } from "@/components/dashboard/fact"
import { useAgentClient } from "@/components/data/clients-query"
import { ResourceMeter } from "@/components/dashboard/resource-meter"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import {
  type ClientSnapshot,
  display,
  formatBytes,
  formatDuration,
  formatPercent,
} from "@/lib/clients"

export const Route = createFileRoute("/agents/$agentId/system")({
  component: SystemTab,
})

function SystemTab() {
  const { agentId } = Route.useParams()
  const client = useAgentClient(agentId)

  if (!client) {
    return null
  }

  return <SystemContent client={client} />
}

function SystemContent({ client }: { client: ClientSnapshot }) {
  const gpuDevices = Array.isArray(client.gpu_devices) ? client.gpu_devices : []
  const addresses = Array.isArray(client.network_addresses)
    ? client.network_addresses
    : []
  const applicationTypes = Array.isArray(client.application_types)
    ? client.application_types
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
          <Fact label="Uptime" value={formatDuration(client.uptime_seconds)} />
          <Fact label="OS" value={display(client.os_version)} />
          <Fact label="Kernel" value={display(client.kernel_version)} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>System Disk</CardTitle>
          <CardDescription>
            Client-authored capacity, filesystem, and cached application
            inventory for the operating-system volume.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <ResourceMeter
            label="Storage Utilization"
            value={client.storage_usage || 0}
            detail={`${formatBytes(client.available_storage_bytes)} available of ${formatBytes(client.total_storage_bytes)}`}
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <Fact
              icon={HardDrive}
              label="Device"
              value={display(client.storage_device)}
              mono
            />
            <Fact label="Model" value={display(client.storage_model)} />
            <Fact label="Media Type" value={display(client.storage_type)} />
            <Fact
              label="Filesystem"
              value={display(client.storage_filesystem)}
            />
            <Fact
              label="Mount Point"
              value={display(client.storage_mountpoint)}
              mono
            />
            <Fact
              label="Mount Mode"
              value={client.storage_read_only ? "Read-only" : "Read/write"}
            />
            <Fact
              label="Used Capacity"
              value={formatBytes(client.used_storage_bytes)}
            />
            <Fact
              label="Inode Utilization"
              value={
                client.storage_inode_usage > 0
                  ? formatPercent(client.storage_inode_usage)
                  : "n/a"
              }
            />
          </div>
          <Separator />
          <div className="flex flex-col gap-3">
            <div>
              <p className="font-medium">Installed Application Mix</p>
              <p className="text-sm text-muted-foreground">
                Most prevalent application types from the bounded,
                process-cached OS inventory.
              </p>
            </div>
            {applicationTypes.length ? (
              <div className="flex flex-wrap gap-2">
                {applicationTypes.map((applicationType) => (
                  <Badge key={applicationType.category} variant="secondary">
                    {applicationType.category}: {applicationType.count}
                  </Badge>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                No installed application categories were reported.
              </p>
            )}
          </div>
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
