# Lingual Standard

## Terminology

The repository uses a constrained vocabulary so documentation remains operationally unambiguous.

- `agent`: the authenticated client process that emits telemetry.
- `gateway`: the ingress service that validates identity and publishes events.
- `broker`: the NATS JetStream transport layer used for downstream event distribution.
- `envelope`: the server-authored metadata wrapper surrounding each accepted payload.
- `payload`: the client-originated telemetry object carried inside the envelope.

## Writing Standard

Documentation should be written in a formal engineering register. Use exact nouns, explicit system boundaries, and implementation-specific language. Avoid casual phrasing, implied behavior, and vague promises.

Preferred characteristics:

- specify the owning component for every behavior;
- distinguish trust metadata from telemetry;
- identify whether a field is client-derived or server-authored;
- state prerequisites, constraints, and failure modes directly.

## Documentation Usage

This file is the canonical reference for naming discipline in the repository. If a term can be confused with a broader industry meaning, the repository-specific meaning takes precedence.
