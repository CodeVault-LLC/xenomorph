# Xenomorph Overview

## System Purpose

Xenomorph is an internal remote screening and support platform for controlled, explicitly authorized environments. The platform consists of an agent, gateway, shared protocol, administrative website, and NATS JetStream dependency. It collects telemetry and supports gateway-mediated command, terminal, screen, log, filesystem, and transfer workflows.

The repository is not release-ready. `.docs/project-status.md` is the authoritative readiness decision.

## Component Ownership

| Component | Owns | Does not own | Trust source |
| --- | --- | --- | --- |
| Agent | Local telemetry, local command execution, filesystem adapters, screen capture, transfer I/O | Identity assertion, operator authorization, command authorship, event provenance | Gateway command verification key and mutually authenticated gateway channel |
| Gateway | Agent authentication, certificate-derived agent ID, server-authored event/command identifiers, command signing and dispatch, validation, audit coordination, broker publication | Truth of client telemetry/results, local filesystem interpretation, current human-operator authentication | Verified client certificate for agent identity; gateway cryptographic state for command authorship |
| Website | Presentation, navigation, accessibility, and collection of operator intent | Agent identity, command authenticity, filesystem truth, server-side authorization | Gateway responses for system state; browser input remains operator-authored |
| Shared protocol | Versioned Go/protobuf and command/file structures | Runtime authorization, persistence, or transport | Reviewed source contracts and generated artifacts |
| NATS JetStream | Acknowledged durable storage/delivery of gateway-published protobuf events | Agent or operator identity, command issuance, payload truth | Gateway broker identity and subject policy once secured; current development connection lacks this production control |

## Runtime Flows

Agent traffic reaches the gateway over TLS 1.3 mutual TLS. The gateway derives the agent ID from the verified certificate fingerprint. It creates trusted envelope metadata and treats heartbeat, entry, log, terminal, screen, filesystem, transfer, and command-result content as client-authored payload.

The gateway creates signed, audience-bound, expiring commands. The agent verifies the dedicated command public key, command type, key ID, audience, time window, and replay nonce before local execution. Results return through the authenticated agent channel but remain client-authored observations.

The website calls the gateway dashboard listener for administrative workflows. Browser requests are operator-authored. The current runtime has no authenticated human operator or authorization middleware, so that listener is a release-blocking administrative trust gap even when bound to loopback or protected by TLS.

The gateway publishes protobuf events synchronously to NATS JetStream and returns success only after the broker acknowledgement. The current broker client accepts a URL without configured TLS credentials or subject authorization. Production broker protection remains a release blocker.

## Persistence and Recovery Boundary

File-operation and transfer records are filesystem-backed under the configured gateway state path. Command queues and several dashboard/runtime stores are memory-backed and can be lost at restart. Client replay state and credentials are local files. These mixed durability semantics are development behavior, not an approved recovery contract.

## Repository Structure

- `platform/client`: Go agent runtime, transport, command handling, telemetry, screen, and filesystem adapters.
- `platform/services/gateway`: Go gateway, mTLS listeners, dashboard API, command queue, key service, file workspace, activity state, and broker integration.
- `platform/shared`: shared command/file contracts and protobuf schema/generated Go artifacts.
- `platform/website`: React and TypeScript administrative website.
- `scripts`: development operational helpers.
- `.docs`: architecture, standards, plans, status, and roadmap.

## Deployment Constraint

The intended Milestone 1 deployment is an authorized internal environment with an authenticated operator boundary, explicit agent enrollment, protected gateway and NATS service identities, externalized secrets, and documented recovery. The present local-development defaults and tracked development certificates are not production deployment inputs.
