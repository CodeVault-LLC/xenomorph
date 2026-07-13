# Xenomorph

Xenomorph is an internal remote screening platform implemented as a Go control plane and Go agent. The repository currently contains three primary modules:

- `platform/services/gateway`: mTLS-terminated ingestion gateway that accepts heartbeat traffic and publishes normalized events into NATS JetStream.
- `platform/client`: agent process that establishes a mutually authenticated TLS session with the gateway and submits heartbeat telemetry.
- `platform/shared`: protocol definitions and generated types shared by both sides of the system.

The repository is intentionally structured for controlled, authorized environments. Any deployment, test harness, or operator workflow must assume explicit administrative authorization and a bounded internal trust domain.

## Operational Model

The current implementation is intentionally narrow. The gateway accepts authenticated client traffic, derives agent identity from the verified client certificate fingerprint, wraps the payload in a trusted envelope, and forwards the event to NATS JetStream. The client is a heartbeat emitter and does not expose a generalized command surface in the present tree.

## Build

Use the repository `Makefile` for repeatable development actions.

```bash
make help
make fmt
make test
make build
make build-all
```

Build artifacts are emitted to `bin/`.

`make build-all` cross-compiles the gateway and client without requiring a Linux,
macOS, or Windows host. It emits Linux (`amd64`, `arm64`), macOS (`amd64`,
`arm64`), and Windows (`amd64`, `arm64`) artifacts below `bin/<os>/<arch>/`.
Cross-compiled binaries are compile-validated; platform-specific behavior must be
tested on its target operating system.

## Go Quality Checks

The Go code is split into client, gateway, and shared-schema modules. Run the
repository targets so every module is checked:

```bash
make install-tools
make test-race
make vet
make staticcheck
make govulncheck
make gosec
make ci
```

The corresponding commands executed per module are `go test ./...`, `go test
-race ./...`, `go vet ./...`, `staticcheck ./...`, `govulncheck ./...`, and
`gosec ./...`. The root module does not own the platform packages, so invoking
those package patterns only at the repository root does not validate the client,
gateway, or shared modules.

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

### Gateway Activity Configuration

Set runtime activity settings with environment variables before launching the gateway:

```bash
export ACTIVITY_OFFLINE_AFTER=30s
export ACTIVITY_SWEEP_INTERVAL=5s
```

The gateway marks an agent online when it receives an authenticated heartbeat and
marks it offline after `ACTIVITY_OFFLINE_AFTER` without a heartbeat. The dashboard
reads this gateway-owned presence state; client-provided hostnames remain telemetry.

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
