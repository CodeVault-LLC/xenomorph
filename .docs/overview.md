# Xenomorph Overview

## System Purpose

Xenomorph is an internal remote screening platform composed of a gateway, an agent, and a shared event schema. The repository models a controlled telemetry path in which the gateway is the sole trust boundary and the agent is an authenticated emitter of operational status.

## Architectural Summary

The current system has three explicit layers:

1. The agent constructs heartbeat telemetry and submits it over mTLS to the gateway.
2. The gateway validates the client certificate, derives the agent identity from the certificate common name, and wraps the payload in a server-authored envelope.
3. The broker publishes the normalized event into NATS JetStream for downstream consumers.

An outbound provider fanout layer consumes trusted heartbeat envelopes and derives activity transitions (`online`, `offline`) per authenticated agent identity. This layer currently includes a Discord provider implementation and is designed to support additional providers without modifying gateway ingress contracts.

This arrangement establishes a single ingress point for identity enforcement, schema validation, and event provenance. Payload data remains distinct from trust metadata.

## Repository Structure

- `platform/client`: agent runtime and client-side HTTP transport.
- `platform/services/gateway`: ingestion service, TLS endpoint, and NATS publishing layer.
- `platform/shared`: protobuf schema and generated Go types.
- `scripts`: operational utilities, currently certificate generation helpers.
- `.docs`: repository documentation for operators, engineers, and automation.

## Runtime Boundaries

The gateway is expected to run inside the protected service plane with reachability to NATS JetStream. The agent is expected to operate in a constrained network domain with access to the gateway certificate authority and the client credential pair. The system should not be treated as an Internet-exposed control surface.

## Control-Plane Contract

The current contract is intentionally conservative:

- TLS client authentication is mandatory.
- Event envelopes are gateway-authored.
- Agent identity is bound to certificate material, not to self-declared payload data.
- Downstream consumers should treat payload fields as telemetry, not as trust evidence.

## Provider Extension Contract

Provider implementations operate on gateway-normalized `ActivityEvent` objects. A provider does not parse client HTTP payloads directly and does not own authentication decisions.

- Owned by gateway: authentication state, `agent_id`, transition derivation, and event timestamp authority.
- Not owned by providers: certificate validation, HTTP ingress, or schema trust decisions.
- Client-authored fields such as hostname remain untrusted telemetry labels.
