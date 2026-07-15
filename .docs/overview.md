# Xenomorph Overview

## System Purpose

Xenomorph is an internal remote screening and support platform for controlled, explicitly authorized environments. The platform consists of an agent, gateway, shared protocol, administrative website, and NATS JetStream dependency. It collects telemetry and supports gateway-mediated command, terminal, screen, log, filesystem, and transfer workflows.

The repository is not release-ready. `.docs/project-status.md` is the authoritative readiness decision. QUIC is the required runtime agent transport, but that selection is not itself a production deployment approval.

## Component Ownership

| Component | Owns | Does not own | Trust source |
| --- | --- | --- | --- |
| Agent | Local telemetry, local command execution, filesystem adapters, screen capture, transfer I/O | Identity assertion, operator authorization, command authorship, event provenance | Gateway command verification key and mutually authenticated gateway channel |
| Gateway | Agent authentication, certificate-derived agent ID, server-authored event/command identifiers, command signing and dispatch, validation, audit coordination, broker publication | Truth of client telemetry/results, local filesystem interpretation, current human-operator authentication | Verified client certificate for agent identity; gateway cryptographic state for command authorship |
| Website | Presentation, navigation, accessibility, and collection of operator intent | Agent identity, command authenticity, filesystem truth, server-side authorization | Gateway responses for system state; browser input remains operator-authored |
| Shared protocol | Versioned XBP agent schema/codecs, Go/protobuf broker contract, and command/file structures | Runtime authorization, persistence, or network identity | Reviewed schema/source contracts and reproducible generated artifacts |
| NATS JetStream | Acknowledged durable storage/delivery of gateway-published protobuf events | Agent or operator identity, command issuance, payload truth | Gateway broker identity and subject policy once secured; current development connection lacks this production control |

## Runtime Flows

Agent traffic reaches the gateway exclusively over raw QUIC v1 with TLS 1.3 mutual authentication, ALPN `xenomorph-agent/1`, bounded reliable streams, and XBP/1. The runtime starts no agent HTTPS/WebSocket listener and the client has no HTTP fallback. The gateway derives the agent ID from the verified certificate fingerprint before application dispatch. It creates trusted envelope metadata and treats hello fields, heartbeat, entry, log, terminal, screen, filesystem, transfer, and command-result content as client-authored payload.

The gateway creates signed, audience-bound, expiring commands. The agent verifies the dedicated command public key, command type, key ID, audience, time window, and replay nonce, then durably reserves replay state before local execution. Results return through the authenticated agent channel but remain client-authored observations. The QUIC command lane pushes the existing JSON-signed envelope; binary command signatures are not approved.

The website calls the gateway dashboard listener for administrative workflows. Browser requests are operator-authored. The current runtime has no authenticated human operator or authorization middleware. Operator authentication is deferred beyond Milestone 1; the listener's security relies on network placement controls.

The gateway publishes protobuf events synchronously to NATS JetStream and returns success only after the broker acknowledgement. The current broker client accepts a URL without configured TLS credentials or subject authorization. Production broker protection remains a release blocker.

## Persistence and Recovery Boundary

File-operation and transfer records are filesystem-backed under the configured gateway state path. The gateway command queue is backed by a bounded command journal and non-command side effects use a bounded operation journal. A dispatch or acceptance interrupted before a terminal commit recovers as `outcome_unknown`, not automatic execution. The client command replay ledger is authenticated by a separate owner-only local key and also recovers interrupted accepted work as `outcome_unknown`. Several dashboard/runtime presentation stores remain memory-backed. These mixed durability semantics are not a complete deployment backup or disaster-recovery approval.

## Repository Structure

- `platform/client`: Go agent runtime, transport, command handling, telemetry, screen, and filesystem adapters.
- `platform/services/gateway`: Go gateway, mTLS listeners, dashboard API, command queue, key service, file workspace, activity state, and broker integration.
- `platform/shared`: canonical XBP agent protocol, generated codecs/reference, shared command/file contracts, and protobuf broker schema/generated Go artifacts.
- `platform/website`: React and TypeScript administrative website.
- `scripts`: development operational helpers.
- `.docs`: architecture, standards, plans, status, and roadmap.

## Deployment Constraint

The intended Milestone 1 deployment is an authorized internal environment with explicit agent enrollment, protected gateway and NATS service identities, externalized secrets, and documented recovery. Operator authentication is deferred beyond Milestone 1. The present local-development defaults and tracked development certificates are not production deployment inputs. QUIC additionally requires an approved UDP/firewall/NAT/kernel profile, external reset/token keys, capacity evidence, and the gates in `.docs/quic-agent-transport-evidence.md`.
