# QUIC Agent Transport Runtime Contract

Status date: 2026-07-15.

Status: implemented as the required agent transport. Production deployment is
not approved without the evidence recorded in the release record.

## Ownership and boundary

`platform/services/gateway/internal/agentquic` owns the UDP socket, QUIC/TLS
termination, pre-authentication admission, negotiated session, stream topology,
replay window, replacement fence, application acknowledgements, metrics, and
bounded qlog output. It does not own payload truth, operator authorization,
durable command intent, file authorization, or broker durability.

`platform/client/internal/agentquic` owns dialing, gateway verification,
negotiation, reconnect scheduling, bounded lane writers, command receipt,
transfer adapters, and reliable media upload. It does not assert trusted agent
identity or command authorship.

`platform/shared/wire` owns canonical XBP bytes, message and stream registries,
semantic bounds, error codes, generated codecs, replay-window mechanics, and
versioned protocol documentation. The canonical input is
`platform/shared/protocol/xbp-v1.yaml`; generated files are never hand-edited.

The gateway obtains the agent ID from the verified certificate before it creates
a session or invokes ingress. No payload field can override that identity.

## Transport profile

The gateway owns one `net.UDPConn` and one `quic.Transport` for the listener
lifetime. Both peers offer only QUIC v1 and `xenomorph-agent/1`, require TLS 1.3,
disable 0-RTT and datagrams, and use bounded stream/connection receive windows.
The client requires an explicit DNS `ServerName`, CA pool, and exactly one client
certificate; `InsecureSkipVerify` is rejected.

Before identity exists, the listener applies a global incomplete-handshake cap,
bounded source-prefix token buckets, optional Retry/address validation,
handshake timeout, certificate-chain byte/depth bounds, and active-session cap.
Source addresses are untrusted operational metadata. Application workers,
session registry installation, command lookup, and broker work begin only after
certificate verification.

The first client-initiated bidirectional stream must be control. `ClientHello`
must be the first frame and must offer minor version 0, a bounded implementation
label, allowlisted platform/architecture, and a nonzero instance nonce. The
gateway returns a random 128-bit session ID, cadence and resource limits, and the
command verification key ID. A mismatch with the client's locally configured
command key is a downgrade-prohibited security failure.

## Stream topology

| Stream | Initiator and direction | Runtime behavior |
| --- | --- | --- |
| Control `0x00` | Client, bidirectional | Exactly one; hello, ping/pong, acknowledgements, drain, replacement. |
| Events `0x01` | Client, unidirectional | Exactly one; heartbeat, attestation, fixed log, command state/result. |
| Commands `0x02` | Gateway, unidirectional | Exactly one; existing signed envelopes are pushed after durable dispatch transition. |
| Transfer `0x03` | Either, bidirectional | One operation per stream; bounded concurrency, signed contract, digest and durable chunk acknowledgements. |
| Terminal `0x04` | Gateway, bidirectional | Reserved; receipt is rejected because no PTY wire contract is approved. |
| Media `0x05` | Client, unidirectional | One active reliable generation; generation, ordering, authorization, and frame bounds are validated. |
| Diagnostics `0x06` | Either, bidirectional | Reserved and disabled in production. |

Every stream begins with the XBP preamble. Initiator, direction, kind, message
type, schema revision, flags, sequence, and operation ID are validated before
dispatch. One session-scoped sliding replay window accepts bounded cross-stream
reordering, rejects duplicates/stale sequences, and rejects pathological gaps.

## Commit and retry semantics

An application acknowledgement describes only the defined commit point. It is
not an execution claim and is not inferred from QUIC packet acknowledgement.

| Family | Successful commit | Retry contract |
| --- | --- | --- |
| Heartbeat | Synchronous NATS acknowledgement | A later sample may supersede an uncommitted sample; no silent republish with a new event ID. |
| Attestation | Durable operation reservation plus synchronous NATS acknowledgement | Retry only with the same operation ID; identical terminal input is duplicate. |
| Fixed client log | Synchronous NATS acknowledgement | No application retry. |
| Command dispatch | Journal state is `dispatched` before frame write | Redelivery is reconciliation only. |
| Command acceptance | Client replay ledger is durable before execution | A restart from accepted becomes `outcome_unknown`. |
| Command result | Audience/type validation, terminal journal transition, and required audit publication | Identical terminal result is duplicate; conflict rejects. |
| Transfer chunk | Existing encrypted workspace persistence and chunk acknowledgement | Same transfer/index/digest is idempotent; conflict rejects. |
| Media | Decode, generation/order/size checks, and exact gateway-issued generation/limit authorization for an active viewer | Stale frames are not application-retried. |

The client replay ledger stores only command ID, nonce digest, key ID, audience,
issued/expiry times, state, terminal digest, and retention. It is authenticated
with a separate owner-only local key. The gateway command and non-command
operation journals are bounded, filesystem-backed, written atomically, and
recover in-flight states as `outcome_unknown`.

## Compatibility

The dashboard remains HTTPS-only. NATS subjects and protobuf `EventEnvelope`
shapes remain unchanged. The application does not start an agent HTTPS listener
and the client contains no HTTP/WebSocket fallback transport. All heartbeat,
attestation, log, command, transfer, and media traffic uses the authenticated
QUIC session.

ALPN major versions are incompatible. Minor version is selected once per
session. Message revisions are exact. Registry IDs are append-only and
tombstoned IDs cannot be reused. Transport and command-signature versions
migrate independently.

## Configuration

The gateway always starts the QUIC agent listener. Failure to bind UDP, load
credentials, or initialize transport state fails gateway startup.

| Gateway setting | Default | Contract |
| --- | ---: | --- |
| `AGENT_QUIC_ADDR` | `:8444` | UDP bind address; separate from HTTPS/dashboard listeners. |
| `AGENT_QUIC_SERVER_CERT_FILE`, `AGENT_QUIC_SERVER_KEY_FILE`, `AGENT_QUIC_CLIENT_CA_FILE` | certificate-path files | External TLS identity and enrollment roots. |
| `AGENT_QUIC_STATELESS_RESET_KEY_FILE`, `AGENT_QUIC_TOKEN_KEY_FILE` | state-path secret files | Loaded when present; otherwise independently generated with the approved CSPRNG and atomically persisted as owner-only files. Each contains exactly 32 nonzero bytes encoded as hex. |
| `AGENT_QUIC_REQUIRE_RETRY` | `false` | Enables source-address validation for the reviewed deployment threat model. |
| `AGENT_QUIC_HANDSHAKE_TIMEOUT`, `AGENT_QUIC_IDLE_TIMEOUT` | `5s`, `45s` | Handshake and maximum idle bounds. |
| `AGENT_QUIC_KEEPALIVE`, `AGENT_QUIC_HEARTBEAT_INTERVAL` | `10s`, `15s` | Keepalive is transport liveness; heartbeat is application telemetry. |
| `AGENT_QUIC_CONTROL_TIMEOUT`, `AGENT_QUIC_DRAIN_TIMEOUT` | `5s`, `5s` | Mandatory-lane negotiation and bounded shutdown. |
| `AGENT_QUIC_TRANSFER_IO_TIMEOUT`, `AGENT_QUIC_MEDIA_FRAME_TIMEOUT` | `60s`, `10s` | Per-I/O slow-peer bounds refreshed only after a complete canonical frame. |
| `AGENT_QUIC_INITIAL_STREAM_WINDOW`, `AGENT_QUIC_MAX_STREAM_WINDOW` | `256 KiB`, `4 MiB` | Per-stream receive windows. |
| `AGENT_QUIC_INITIAL_CONNECTION_WINDOW`, `AGENT_QUIC_MAX_CONNECTION_WINDOW` | `512 KiB`, `16 MiB` | Aggregate connection receive windows. |
| `AGENT_QUIC_MAX_BIDI_STREAMS`, `AGENT_QUIC_MAX_UNI_STREAMS` | `8`, `4` | Incoming stream caps. |
| `AGENT_QUIC_MAX_HANDSHAKES`, `AGENT_QUIC_MAX_SESSIONS`, `AGENT_QUIC_MAX_REGISTRY_ENTRIES` | `128`, `1000`, `2000` | Global admission and registry bounds. |
| `AGENT_QUIC_SOURCE_RATE`, `AGENT_QUIC_SOURCE_BURST`, `AGENT_QUIC_MAX_SOURCE_PREFIXES` | `5/s`, `20`, `4096` | Pre-identity abuse controls, never authorization. |
| `AGENT_QUIC_MAX_CLIENT_CHAIN_BYTES`, `AGENT_QUIC_MAX_CLIENT_CHAIN_DEPTH` | `32 KiB`, `5` | Presented certificate-chain bounds. |
| `AGENT_QUIC_MAX_TRANSFERS`, `AGENT_QUIC_EVENT_FRAME_MAX` | `4`, `256 KiB` | Application concurrency/frame bounds. |
| `AGENT_QUIC_DIAGNOSTICS_ENABLED`, `AGENT_QUIC_DIAGNOSTIC_PATH` | `false`, state-path `qlog` | Explicit owner-only diagnostic capture. |
| `AGENT_QUIC_DIAGNOSTIC_FILE_LIMIT`, `AGENT_QUIC_DIAGNOSTIC_BYTE_LIMIT` | `8`, `64 MiB` | Aggregate retained-file and byte ceilings, including existing qlog files. |

The client always requires QUIC. `AGENT_QUIC_ENDPOINT` defaults to
`localhost:8444`; `AGENT_OPERATION_TIMEOUT` defaults to `10s`. TLS name,
credentials, CA, command key, ledger/key paths, heartbeat, QUIC
timeouts/keepalive, and reconnect backoff are externalized through the
`AGENT_*` settings defined in `platform/client/internal/config/config.go`.
Production rejects `localhost` as the TLS name.
Legacy `AGENT_TRANSPORT_MODE`, `AGENT_GATEWAY_URL`, `AGENT_HTTP_TIMEOUT`, and
`AGENT_HTTP_FALLBACK_UNTIL` settings are rejected rather than ignored.

## Generated protocol workflow

Run `make wire-generate` after changing the canonical schema and commit the
schema, registry history, generated Go, wire reference, and vectors together.
`make wire-generate-check` verifies deterministic output. `make ci-go` runs this check,
all module tests, race/vet/static/security/vulnerability/lint gates, and client
cross-builds.
