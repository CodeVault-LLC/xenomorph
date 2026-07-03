import { Activity, Clock, Cpu } from "lucide-react"

import { TermLink } from "@/components/glossary/term-link"
import { MetricCard } from "@/components/dashboard/metric-card"
import { ResourceMeter } from "@/components/dashboard/resource-meter"
import {
  type ClientSnapshot,
  formatPercent,
  formatRelative,
  resourceTone,
} from "@/lib/clients"

export function AgentAnalytics({ client }: { client: ClientSnapshot }) {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 md:grid-cols-3">
        <MetricCard
          icon={Cpu}
          label="CPU Load"
          value={formatPercent(client.cpu_load)}
          tone={resourceTone(client.cpu_load)}
        />
        <MetricCard
          icon={Activity}
          label="RAM Usage"
          value={formatPercent(client.ram_usage)}
          tone={resourceTone(client.ram_usage)}
        />
        <MetricCard
          icon={Clock}
          label="Last Seen"
          value={formatRelative(client.last_seen)}
          tone="default"
        />
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <ResourceMeter label="CPU Pressure" value={client.cpu_load} />
        <ResourceMeter label="Memory Pressure" value={client.ram_usage} />
      </div>

      <p className="text-xs text-muted-foreground">
        Pressure thresholds are defined in the{" "}
        <TermLink slug="cpu-load">CPU</TermLink> and{" "}
        <TermLink slug="ram-usage">RAM</TermLink> entries.
      </p>
    </div>
  )
}
