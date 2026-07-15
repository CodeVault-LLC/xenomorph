# Xenomorph

Xenomorph is an internal remote screening and support platform implemented as a Go control plane, Go agent, shared protocol, and React administrative website. The primary components are:

- `platform/services/gateway`: mTLS-terminated agent boundary, command author, dashboard API, file-workspace coordinator, and NATS publisher.
- `platform/client`: agent process for telemetry and gateway-authored command execution.
- `platform/shared`: protocol definitions and generated types shared across the Go components.
- `platform/website`: React and TypeScript administrative interface.

The repository is intentionally structured for controlled, authorized environments. Any deployment, test harness, or operator workflow must assume explicit administrative authorization and a bounded internal trust domain.

## Current Status

The project has a substantial working development baseline, including signed command handling, telemetry, terminal, screen, logs, and a gateway-mediated file workspace. It is **not ready for release**. Protected broker transport, credential remediation, cryptographic promotion evidence, native platform validation, recovery, and signed release automation remain blocking. See [Project Status](.docs/project-status.md) and [Roadmap](.docs/roadmap.md).

## Build

Use the repository `Makefile` for repeatable development actions.

```bash
make help
make fmt
make test
make build
make build-all
make ci-web
make ci
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

The agent plane uses QUIC exclusively. Start the local NATS dependency first
with `docker compose -f platform/infrastructure/docker-compose.yml up -d nats`.
`make run-gateway` supplies the current Go operating environment to the
development cryptographic-provider allowlist, creates persistent owner-only
QUIC reset/token keys when absent, and listens on UDP `:8444`. Then
`make run-client` connects with the development certificates. The dashboard
remains HTTPS on `127.0.0.1:8080`; no agent HTTPS, WebSocket, or HTTP fallback
listener is started. These defaults are not a production credential contract.

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
- [Project Status](.docs/project-status.md)
- [Roadmap](.docs/roadmap.md)
- [CI, Build, and Release Contract](.docs/ci-and-release.md)
- [Code Review Standard](.docs/code-review.md)
- [Lingual Standard](.docs/lingual.md)

The root [AGENTS.md](AGENTS.md) file defines the operating rules for contributors and automation.

## Security Posture

The system is designed around explicit identity, mTLS, and event normalization at the gateway boundary. The protocol envelope records trust decisions at ingress, while untrusted telemetry remains payload data. Security-sensitive changes must preserve that separation and must not weaken transport authentication, certificate handling, or event provenance.

## License

See [LICENSE](LICENSE) for licensing terms.
