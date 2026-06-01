# Xenomorph

Xenomorph is an internal remote screening platform implemented as a Go control plane and Go agent. The repository currently contains three primary modules:

- `platform/services/gateway`: mTLS-terminated ingestion gateway that accepts heartbeat traffic and publishes normalized events into NATS JetStream.
- `platform/client`: agent process that establishes a mutually authenticated TLS session with the gateway and submits heartbeat telemetry.
- `platform/shared`: protocol definitions and generated types shared by both sides of the system.

The repository is intentionally structured for controlled, authorized environments. Any deployment, test harness, or operator workflow must assume explicit administrative authorization and a bounded internal trust domain.

## Operational Model

The current implementation is intentionally narrow. The gateway accepts authenticated client traffic, derives agent identity from the verified client certificate fingerprint, wraps the payload in a trusted envelope, and forwards the event to NATS JetStream. The client is a heartbeat emitter and does not expose a generalized command surface in the present tree.

The gateway now includes a provider fanout layer for outbound activity notifications. Heartbeat envelopes are converted into server-authored activity events (online and offline transitions) and dispatched to configured providers. Discord is the first provider and the integration surface is intentionally generic so new providers can be added without changing the ingestion contract.

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

### Discord Provider Configuration

Set provider and runtime settings with environment variables before launching the gateway:

```bash
export NOTIFY_PROVIDERS=discord
export DISCORD_BOT_TOKEN=<bot-token>
export DISCORD_GUILD_ID=<guild-id>
```

Optional settings:

```bash
export DISCORD_API_BASE_URL=https://discord.com/api/v10
export ACTIVITY_OFFLINE_AFTER=30s
export ACTIVITY_SWEEP_INTERVAL=5s
```

Behavioral contract:

- `online` notifications are emitted on first authenticated heartbeat from an agent, and when the same agent returns after being marked offline.
- `offline` notifications are emitted when no heartbeat has been seen for `ACTIVITY_OFFLINE_AFTER`.
- Provider payloads use gateway-authored identity (`security.agent_id`) derived as a deterministic UUID from the authenticated client certificate. Client-provided hostname is collected at runtime via `os.Hostname()` and remains telemetry-only.
- If `DISCORD_GUILD_ID` is configured, the provider provisions a per-agent Discord workspace automatically:
	- Category format: `client-<hostname-slug>-<uuid-prefix>`
	- Text channels: `audit`, `command`, `uploads`
	- Channels are reused on reconnect using agent UUID markers to avoid duplicates when hostnames collide.

Startup contract:

- When Discord is enabled, gateway startup performs a provider preflight check against Discord (`/users/@me` and `/channels/{id}`) to validate bot authentication and channel access before ingest processing starts.
- If the bot token is invalid or the bot does not have access to the configured channel, startup fails immediately with a detailed error.

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
