import { ShieldCheck } from "lucide-react"

import { Fact } from "@/components/dashboard/fact"
import { TermLink } from "@/components/glossary/term-link"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { type ClientSnapshot, display, formatDate } from "@/lib/clients"

export function AgentDetails({ client }: { client: ClientSnapshot }) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Observed Details</CardTitle>
          <CardDescription>Current gateway view of this agent.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <Fact
            label="Status"
            value={client.is_online ? "Online" : "Offline"}
          />
          <Fact label="Hostname" value={display(client.hostname)} />
          <Fact label="Client IP" value={display(client.client_ip)} mono />
          <Fact label="OS" value={display(client.os_version)} />
          <Fact label="First Seen" value={formatDate(client.first_seen)} />
          <Fact label="Last Online" value={formatDate(client.last_online)} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="size-4 text-muted-foreground" />
            Trust Source
          </CardTitle>
          <CardDescription>
            How to read each field&apos;s trust source.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <p className="text-muted-foreground">
            Identity fields (<TermLink slug="agent-id">Agent ID</TermLink>,{" "}
            <TermLink slug="client-ip">Client IP</TermLink>, first/last seen,
            online state) are <TermLink slug="gateway">gateway</TermLink>
            -authored.
          </p>
          <p className="text-muted-foreground">
            <TermLink slug="telemetry">Telemetry</TermLink> (hostname, OS, CPU,
            RAM, screen) is agent-authored and not trust-bearing.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
