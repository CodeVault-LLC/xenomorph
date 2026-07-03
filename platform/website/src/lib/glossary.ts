export type GlossaryTerm = {
  slug: string
  term: string
  summary: string
  detail: string
  category: "identity" | "transport" | "telemetry" | "state"
}

export const glossary: GlossaryTerm[] = [
  {
    slug: "agent",
    term: "Agent",
    summary:
      "A remote process enrolled with the gateway over mutual TLS and identified by a stable agent_id.",
    detail:
      "Agents are the only producers of client-side data in this system. They connect through mTLS, present a gateway-issued credential, and push heartbeats plus telemetry. The gateway owns their identity; the browser never trusts an agent directly.",
    category: "identity",
  },
  {
    slug: "client",
    term: "Client",
    summary:
      "The mTLS connection endpoint. Used interchangeably with agent when describing what the gateway observes.",
    detail:
      "'Client' is a transport view of an agent: the side of the mTLS connection the gateway terminates. Client fields such as client_ip are gateway-authored; agent fields such as hostname are agent-authored.",
    category: "transport",
  },
  {
    slug: "gateway",
    term: "Gateway",
    summary:
      "The ingestion service that terminates mTLS, owns agent identity, and exposes this UI's API surface.",
    detail:
      "The gateway is the trust boundary for this UI. Anything shown here is gateway state derived from authenticated agent connections. Browsers talk only to the gateway, never to an agent directly.",
    category: "transport",
  },
  {
    slug: "mtls",
    term: "mTLS",
    summary:
      "Mutual TLS. The trust source for every agent identity asserted in this UI.",
    detail:
      "Both sides of the connection present a certificate. An agent that fails certificate validation is rejected before any of its data reaches gateway state.",
    category: "transport",
  },
  {
    slug: "trust-boundary",
    term: "Trust boundary",
    summary:
      "Browser views read gateway state only; agents enter through mTLS ingestion.",
    detail:
      "Telemetry fields (hostname, OS, CPU, RAM, screen) are agent-authored and not trust-bearing. Identity-adjacent fields (agent_id, client_ip, first_seen, online state) are gateway-authored and derived from the mTLS session.",
    category: "transport",
  },
  {
    slug: "agent-id",
    term: "Agent ID",
    summary: "Stable identifier assigned by the gateway at enrollment.",
    detail:
      "Used in URLs and as the primary key across the clients table and agent view. Not the OS hostname and not user-settable.",
    category: "identity",
  },
  {
    slug: "hostname",
    term: "Hostname",
    summary: "OS hostname reported by the agent. Not a trust source.",
    detail:
      "Agent-authored. Displayed for orientation only; identity is keyed on agent_id, not hostname.",
    category: "telemetry",
  },
  {
    slug: "client-ip",
    term: "Client IP",
    summary: "Source address of the agent's mTLS connection, gateway-authored.",
    detail:
      "Reflects the network position the gateway saw on the most recent accepted connection. Not a stable identity.",
    category: "transport",
  },
  {
    slug: "os-version",
    term: "OS Version",
    summary: "Agent-reported os_version string (family and version).",
    detail:
      "Agent-authored telemetry; not used for any authorization decision.",
    category: "telemetry",
  },
  {
    slug: "heartbeat",
    term: "Heartbeat",
    summary: "Periodic signal an agent pushes to the gateway.",
    detail:
      "Agents are considered online while inside the heartbeat window and offline once the window lapses without an accepted signal.",
    category: "state",
  },
  {
    slug: "online",
    term: "Online / Offline",
    summary: "Whether the agent is inside the heartbeat window right now.",
    detail:
      "Online means the gateway has accepted a recent heartbeat for this agent_id. Offline means the agent has been seen before but the window has lapsed.",
    category: "state",
  },
  {
    slug: "known-clients",
    term: "Known clients",
    summary: "Distinct agents the gateway has observed this process lifetime.",
    detail:
      "Includes both online and offline agents. Resets when the gateway process restarts.",
    category: "state",
  },
  {
    slug: "first-seen",
    term: "First Seen",
    summary: "First time the gateway saw this agent during this process.",
    detail: "Gateway-authored. Resets with the gateway process.",
    category: "state",
  },
  {
    slug: "last-seen",
    term: "Last Seen",
    summary: "Most recent heartbeat the gateway accepted for this agent.",
    detail: "Updates on every accepted heartbeat, online or recently offline.",
    category: "state",
  },
  {
    slug: "last-online",
    term: "Last Online",
    summary: "Last timestamp the agent was within the heartbeat window.",
    detail:
      "Distinct from Last Seen: a lapsed heartbeat moves the agent offline.",
    category: "state",
  },
  {
    slug: "cpu-load",
    term: "CPU Load",
    summary: "Utilization across reported cores, agent-reported.",
    detail:
      "Normalized by available CPU cores on the agent host. Not trusted by the gateway.",
    category: "telemetry",
  },
  {
    slug: "ram-usage",
    term: "RAM Usage",
    summary: "Used memory ratio, agent-reported.",
    detail:
      "Derived from the agent heartbeat. Above 85% indicates low headroom.",
    category: "telemetry",
  },
  {
    slug: "telemetry",
    term: "Telemetry",
    summary: "Client-reported data: hostname, OS, CPU, RAM, screen.",
    detail:
      "Convenience data for operators. Identity and authorization never use telemetry fields.",
    category: "telemetry",
  },
]

export const glossaryBySlug = new Map(glossary.map((term) => [term.slug, term]))

export function glossaryTerm(slug: string): GlossaryTerm | undefined {
  return glossaryBySlug.get(slug)
}
