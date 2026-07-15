# Client Logging and Replay Security-State Contract

## Ownership and retention

The client creates fixed operational event metadata. It does not own diagnostic
log storage, retry queues, local log files, registry entries, gateway event IDs,
accepted timestamps, provenance, or downstream retention. In HTTP mode it sends
the existing bounded JSON log request. In QUIC mode it sends the assigned XBP
level, component, and event codes. The gateway derives identity from mTLS,
normalizes the client-authored event, authors its protobuf envelope, and returns
success only after synchronous broker acknowledgement.

A failed log submission is discarded. The client does not retry it, buffer it,
or emit a second failure log. It writes no diagnostic logs to disk. At startup
it removes the retired `~/.xenomorph/agent-state.json`; failure to remove that
legacy diagnostic state stops startup.

## Trust and data classification

Log level, component, event code, and optional approved detail are
client-authored operational metadata. They are not identity, authorization,
host-integrity, or execution evidence. The gateway-authored agent ID comes only
from the verified client certificate. Event ID, session ID, accepted timestamp,
broker subject, and authentication result are server-authored.

Operator commands and payloads remain operator-authored input in the command
plane and are never copied into client logs.

## Fixed event registry

The client emits only assigned events. XBP does not accept arbitrary raw error
or message strings.

| Component | Level | Event | Condition |
| --- | --- | --- | --- |
| `client.runtime` | `INFO` | `runtime_started` | Setup completed. |
| `client.authentication` | `INFO` | `authentication_succeeded` | Initial authenticated heartbeat committed. |
| `client.authentication` | `ERROR` | `authentication_failed` | Initial authenticated heartbeat failed. |
| `client.attestation` | `INFO` | `attestation_submitted` | Endpoint inventory committed. |
| `client.attestation` | `ERROR` | `attestation_failed` | Endpoint inventory failed. |
| `client.heartbeat` | `ERROR` | `heartbeat_failed` | Periodic heartbeat failed. Successful periodic heartbeats are not logged. |
| `client.command` | `INFO` | `command_received` | A signed command was received. |
| `client.command` | `INFO` | `command_completed` | The terminal result was accepted. |
| `client.command` | `ERROR` | `command_poll_failed` | The command transport failed. |
| `client.command` | `ERROR` | `command_processing_failed` | Processing failed before completion. |
| `client.command` | `ERROR` | `command_result_submission_failed` | Terminal result submission failed. |
| `client.runtime` | `ERROR` | `runtime_loop_failed` | A runtime lane exited with error. |

Fallback is not emitted for certificate, server-name, ALPN/version,
command-key, or enforced-protocol failure because those conditions stop startup
instead of downgrading.

## Prohibited diagnostic data

Client logs must not contain certificates, keys, tokens, passwords, environment
variables, network addresses, full hostnames, browser inventory, screenshots,
file content or paths, command payloads, terminal commands/output, raw errors,
request/response bodies, or replay-ledger contents. Peer close descriptions and
public acknowledgement errors are fixed bounded codes.

## Replay ledger is not a diagnostic log

The command replay ledger is mandatory security state for the command trust
boundary. Its existence does not authorize diagnostic persistence. It stores
only command ID, nonce digest, command key ID, audience, issued/expiry window,
state, terminal-result digest, and retention deadline. It never stores command
payloads, reasons, terminal data, paths, screenshots, logs, credentials, or raw
results.

The ledger is authenticated with a separate random 32-byte local key. State,
key, directories, and atomic temporary files use owner-only permissions. The
key is not derived from the TLS private key. A command is durably `accepted`
before execution and terminal state is persisted afterward. Restart while
accepted changes the entry to `outcome_unknown`; the command is not silently
executed again. Capacity, authentication, corruption, or persistence failure
fails closed for new side-effecting commands.

Back up and restore the ledger and authentication key as one security-state
unit through an approved protected channel. Restoring only one causes startup
authentication failure. Credential renewal does not rewrite existing entries;
new commands remain bound to their signed key ID and certificate-derived
audience. Re-enrollment or secure deletion requires an operations-owned
procedure that preserves incident retention and prevents old signed commands
from being accepted under a new identity. The runtime does not claim secure
erasure merely by deleting a filesystem path.
