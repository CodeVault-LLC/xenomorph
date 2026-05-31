# Roadmap

## Near-Term Objectives

1. Formalize service configuration.
   - Introduce explicit environment-driven configuration for gateway address, broker address, certificate locations, and runtime timeouts.
   - Remove hard-coded runtime paths from service entrypoints.

2. Strengthen event handling.
   - Expand the shared protobuf contract with versioned envelopes and explicit validation rules.
   - Introduce server-side schema checks before any message is admitted to the broker.

3. Improve operational transparency.
   - Add structured logging for ingress, broker publication, and shutdown paths.
   - Add observable health and readiness endpoints for service orchestration.

4. Establish delivery discipline.
   - Keep the repository-level `Makefile` as the authoritative developer entrypoint.
   - Add repeatable CI checks for formatting, module tidiness, compilation, and schema consistency.

## Security Considerations

The security model depends on a small set of non-negotiable properties:

- Mutual TLS remains the primary identity primitive.
- Certificate validation must remain strict and deterministic.
- Gateway-authored metadata must never be replaced by client-supplied assertions.
- Broker subjects must remain namespaced and policy-driven.
- Secret material, certificate chains, and environment credentials must be externalized from source control.

The repository should be treated as an internal system for authorized remote screening only. Documentation, deployment, and test procedures must preserve that constraint and must not suggest unrestricted or opportunistic use.

## Feature Trajectory

Expected subsequent work should focus on controlled operator workflows rather than ad hoc client behavior. The likely progression is:

- configuration normalization;
- stronger schema validation;
- lifecycle management for sessions and identities;
- audit-grade telemetry handling;
- hardened certificate and secret management;
- reproducible packaging and release automation.

## Implementation Notes

The present codebase already establishes the key trust boundary: the gateway terminates authenticated transport, assigns identity, and forwards normalized events. Any future feature should preserve that boundary and should not bypass the gateway for convenience.
