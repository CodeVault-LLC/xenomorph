# Xenomorph

Xenomorph is an internal remote screening platform implemented as a Go control plane and Go agent. The repository currently contains three primary modules:

- `platform/services/gateway`: mTLS-terminated ingestion gateway that accepts heartbeat traffic and publishes normalized events into NATS JetStream.
- `platform/client`: agent process that establishes a mutually authenticated TLS session with the gateway and submits heartbeat telemetry.
- `platform/shared`: protocol definitions and generated types shared by both sides of the system.

The repository is intentionally structured for controlled, authorized environments. Any deployment, test harness, or operator workflow must assume explicit administrative authorization and a bounded internal trust domain.

## Operational Model

The current implementation is intentionally narrow. The gateway accepts authenticated client traffic, extracts agent identity from the client certificate subject, wraps the payload in a trusted envelope, and forwards the event to NATS JetStream. The client is a heartbeat emitter and does not expose a generalized command surface in the present tree.

## Build

Use the repository `Makefile` for repeatable development actions.

```bash
make help
make fmt
make test
make build
```

Build artifacts are emitted to `bin/`.

## Local Run Workflow

Run the gateway and client in separate terminals so the control plane stays available before the agent starts emitting telemetry.

Terminal 1:

```bash
make run-gateway
```

Terminal 2:

```bash
make run-client
```

The gateway expects a reachable NATS JetStream endpoint at `nats://localhost:4222` and the certificate material under `platform/infrastructure/certs`. The client expects the same certificate material and a live gateway listener at `https://localhost:8443`.

## Documentation

Repository documentation is maintained in [.docs](.docs):

- [Overview](.docs/overview.md)
- [Roadmap](.docs/roadmap.md)
- [Lingual Standard](.docs/lingual.md)

The root [AGENTS.md](AGENTS.md) file defines the operating rules for contributors and automation.

## Security Posture

The system is designed around explicit identity, mTLS, and event normalization at the gateway boundary. The protocol envelope records trust decisions at ingress, while untrusted telemetry remains payload data. Security-sensitive changes must preserve that separation and must not weaken transport authentication, certificate handling, or event provenance.

## License

See [LICENSE](LICENSE) for licensing terms.
