# ADR 0001: QUIC Agent Transport and XBP Candidate Protocol

Status date: 2026-07-15.

Status: **accepted for disabled-by-default implementation and controlled evaluation; not approved for production enablement or fleet cutover**.

Owners: architecture, gateway security, client runtime, and shared protocol.

## Context

The existing agent plane uses independent HTTPS requests, command polling,
WebSocket media, and HTTP transfer routes. This prevents immediate command push
and couples unrelated traffic to multiple connection lifecycles. The gateway
must remain the only identity, authorization, provenance, and broker-publication
boundary while the transport changes.

## Decision

- Use raw QUIC v1 through `quic-go` with ALPN `xenomorph-agent/1`.
- Require TLS 1.3 and a verified client certificate. The gateway derives the
  agent ID only from the verified leaf certificate.
- Disable TLS/QUIC 0-RTT and QUIC datagrams.
- Maintain one active connection per certificate-derived agent ID. A newly
  authenticated session is installed atomically and fences the previous session
  before it becomes command-eligible.
- Use long-lived control, event, and command streams plus bounded independent
  transfer and reliable media streams. Terminal and diagnostic stream IDs remain
  reserved and disabled.
- Keep the dashboard HTTPS API and gateway-to-NATS protobuf contract unchanged.
- Keep the existing JSON-signed command envelope for the first transport
  migration. Binary command signatures require a separate decision and overlap
  plan.
- Treat XBP/1 as the implemented candidate binary application protocol. Its
  schema, registry, generator, bounded codecs, vectors, and fuzz targets are
  repository-owned. Production use remains conditional on the codec evidence
  gate in `.docs/quic-agent-transport-evidence.md`.
- Keep the gateway QUIC listener disabled by default. Keep HTTP/WebSocket agent
  routes during a bounded compatibility window.
- Permit HTTP fallback only in explicit `quic-first` client mode with a future
  expiry. Never fall back after certificate, server-name, ALPN/version,
  command-key, or minimum-protocol failure.
- Support many agents on one gateway instance. Multi-instance ownership is not
  supported until a shared session lease, command claim, operation journal, and
  connection-ID routing design are approved.

## Trust and data classification

TLS supplies authenticated transport integrity and confidentiality. The client
certificate supplies gateway-trusted agent identity only after verification.
Command signatures supply gateway authorship and audience binding. XBP headers,
hello fields, telemetry, results, media, transfer bytes, remote addresses, and
client timestamps remain client-authored. The gateway authors session IDs,
event IDs, accepted timestamps, broker subjects, and protobuf provenance.

## Consequences

The deployment needs a separately exposed UDP port, persistent confidential
stateless-reset and token keys, UDP observability, drain behavior, and explicit
capacity limits. QUIC improves multiplexing and command latency but adds a
stateful UDP failure domain. XBP adds permanent schema-governance and parser
maintenance costs. Failure between an operating-system side effect and a
durable terminal result remains `outcome_unknown`; the system does not claim
exactly-once execution.

## Production gates

Production enablement requires architecture and gateway-security approval of
the exact release, dependency/FIPS boundary, capacity profile, certificate
lifecycle, UDP deployment path, benchmark result, mixed-version rollback, and
native platform evidence. Fleet cutover additionally requires a canary window,
zero required old-client traffic, fallback disabled, and a separately reviewed
removal release. These approvals are external evidence and are not inferred
from code or this ADR.
