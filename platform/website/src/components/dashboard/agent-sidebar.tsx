import { Link } from "@tanstack/react-router"
import { ArrowLeft, Clock, Fingerprint, Globe2, HardDrive } from "lucide-react"

import { Fact } from "@/components/dashboard/fact"
import { OSBadge } from "@/components/dashboard/os-badge"
import { StatusBadge } from "@/components/dashboard/status-badge"
import { Card } from "@/components/ui/card"
import { type ClientSnapshot, display, formatRelative } from "@/lib/clients"
import { cn } from "@/lib/utils"

export function AgentSidebar({ client }: { client: ClientSnapshot }) {
  return (
    <aside className="flex flex-col gap-4">
      <Link
        to="/"
        className={cn(
          "inline-flex h-8 w-fit shrink-0 items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-2.5 text-sm font-medium whitespace-nowrap transition-all outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
        )}
      >
        <ArrowLeft />
        Clients
      </Link>

      <Card className="p-4">
        <div className="flex flex-wrap gap-2">
          <StatusBadge online={client.is_online} />
          <OSBadge value={client.os_version} detailed />
        </div>
        <h1 className="mt-4 text-xl font-semibold tracking-normal">
          {display(client.hostname)}
        </h1>
        <p className="mt-1 font-mono text-xs break-all text-muted-foreground">
          {client.agent_id}
        </p>
      </Card>

      <Card className="divide-y divide-border">
        <Fact
          variant="row"
          icon={Fingerprint}
          label="Identity"
          value="Gateway mTLS"
        />
        <Fact
          variant="row"
          icon={Globe2}
          label="IP"
          value={display(client.client_ip)}
          mono
        />
        <Fact
          variant="row"
          icon={Clock}
          label="Last seen"
          value={formatRelative(client.last_seen)}
        />
        <Fact
          variant="row"
          icon={HardDrive}
          label="Telemetry"
          value="Client reported"
        />
      </Card>
    </aside>
  )
}
