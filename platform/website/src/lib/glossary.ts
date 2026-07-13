export type GlossaryCategory = "identity" | "transport" | "telemetry" | "state"

export type GlossarySource =
  "Gateway-authored" | "Agent-authored" | "Transport control"

export type GlossaryTerm = {
  slug: string
  term: string
  summary: string
  detail: string
  category: GlossaryCategory
  source: GlossarySource
}

export const glossary: GlossaryTerm[] = [
  {
    slug: "gateway",
    term: "Gateway",
    summary:
      "The control-plane service that authenticates agents, derives identity, and supplies dashboard state.",
    detail:
      "The gateway is the system trust boundary. The dashboard reads gateway state and sends actions through the gateway; it never establishes trust directly with an agent.",
    category: "transport",
    source: "Gateway-authored",
  },
  {
    slug: "mtls",
    term: "Mutual TLS (mTLS)",
    summary:
      "The authenticated transport required before the gateway accepts an agent connection.",
    detail:
      "Both endpoints present and validate certificates. A connection that fails validation cannot create agent state or submit telemetry.",
    category: "transport",
    source: "Transport control",
  },
  {
    slug: "trust-boundary",
    term: "Trust boundary",
    summary:
      "The point where the gateway converts an authenticated connection into authoritative system state.",
    detail:
      "Only gateway-derived fields may support identity or authorization decisions. Agent-supplied values remain telemetry after they cross the boundary.",
    category: "transport",
    source: "Transport control",
  },
  {
    slug: "agent-id",
    term: "Agent ID",
    summary: "Gateway-derived identifier for the enrolled agent record.",
    detail:
      "This is the stable record key used in dashboard routes and gateway state. It is not the hostname and is not supplied by the host operating system.",
    category: "identity",
    source: "Gateway-authored",
  },
  {
    slug: "client-ip",
    term: "Client IP",
    summary:
      "Source address observed by the gateway on the most recent accepted connection.",
    detail:
      "It describes current network position, not a durable identity. Address changes do not create a new agent record by themselves.",
    category: "identity",
    source: "Gateway-authored",
  },
  {
    slug: "heartbeat",
    term: "Heartbeat",
    summary:
      "An accepted liveness signal from an authenticated agent connection.",
    detail:
      "The gateway uses accepted heartbeats to refresh liveness state. A received signal is not accepted until transport validation and gateway processing succeed.",
    category: "state",
    source: "Gateway-authored",
  },
  {
    slug: "online",
    term: "Online / offline",
    summary:
      "Whether the gateway has accepted a heartbeat within the configured liveness window.",
    detail:
      "Online does not assert host health beyond recent gateway contact. Offline means the record is known but no accepted heartbeat remains inside the window.",
    category: "state",
    source: "Gateway-authored",
  },
  {
    slug: "first-seen",
    term: "First observed",
    summary:
      "The first accepted observation of this agent during the current gateway process lifetime.",
    detail:
      "This timestamp is gateway-authored and resets when gateway in-memory state is restarted. It is not an enrollment or operating-system installation time.",
    category: "state",
    source: "Gateway-authored",
  },
  {
    slug: "last-seen",
    term: "Last heartbeat",
    summary:
      "The most recent heartbeat the gateway accepted for the agent record.",
    detail:
      "Use it to assess recency of gateway contact. It may be older than the current view while an agent is offline.",
    category: "state",
    source: "Gateway-authored",
  },
  {
    slug: "last-online",
    term: "Last online",
    summary:
      "The latest time the agent record was inside the gateway liveness window.",
    detail:
      "This records the end of the most recent known-online period and is distinct from the timestamp of a heartbeat currently being processed.",
    category: "state",
    source: "Gateway-authored",
  },
  {
    slug: "telemetry",
    term: "Telemetry",
    summary: "Operational data supplied by the agent for operator awareness.",
    detail:
      "Hostname, operating-system information, hardware inventory, resource values, screen media, and logs are telemetry. The gateway does not use them as evidence of identity or authorization.",
    category: "telemetry",
    source: "Agent-authored",
  },
  {
    slug: "cpu-load",
    term: "CPU load",
    summary:
      "Current CPU utilization normalized by the agent's reported available cores.",
    detail:
      "Use this as an operational pressure signal, not an integrity assertion. Collection and normalization occur on the agent host.",
    category: "telemetry",
    source: "Agent-authored",
  },
  {
    slug: "ram-usage",
    term: "Memory usage",
    summary: "Current used-memory ratio reported by the agent.",
    detail:
      "This indicates reported memory pressure at the time of collection. It does not establish a resource guarantee or authorize an action.",
    category: "telemetry",
    source: "Agent-authored",
  },
]

export const glossaryBySlug = new Map(glossary.map((term) => [term.slug, term]))

export function glossaryTerm(slug: string): GlossaryTerm | undefined {
  return glossaryBySlug.get(slug)
}
