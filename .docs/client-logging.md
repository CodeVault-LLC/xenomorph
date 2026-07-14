# Client Remote Logging Contract

## Ownership and Retention

The client owns the creation of operational log records. It does not own log
storage, log retry queues, local files, operating-system registry entries, or
log delivery guarantees. Each record is sent directly over the existing mTLS
connection to `POST /ingest/logs`. The gateway owns authentication, event IDs,
timestamps, client IP attribution, broker publication, dashboard retention,
and any downstream storage policy.

The client writes no diagnostic logs to disk. At startup it removes the legacy
`~/.xenomorph/agent-state.json` file, if present, and never recreates it. It
creates no log directory, no database, and no registry entry. A failed log
submission is discarded. The client does not retry it, buffer it, or report a
second log about the failure. A legacy state file that cannot be removed stops
client startup so that the client does not operate while leaving the retired
data at rest.

## Trust and Data Classification

The client presents its mTLS certificate but does not assert its own identity.
The gateway derives the agent ID from the verified client certificate and
creates the event envelope. That agent ID, event ID, timestamp, client IP, and
authentication result are server-authored. The `level`, `component`, and
`message` fields are client-authored operational metadata. They are not
identity evidence, authorization evidence, or host-integrity evidence.

No log field is operator-authored. Operator commands and their payloads remain
operator-authored input in the command plane and must not be copied into a log
record.

## Wire Shape

The client sends the following JSON body. The gateway normalizes and bounds
the fields before publishing it.

```json
{
  "level": "INFO",
  "component": "client.command",
  "message": "event=command_completed"
}
```

`level`, `component`, and `message` are client-authored. The gateway accepts
only `DEBUG`, `INFO`, `WARN`, and `ERROR`, limits level length to 16 bytes,
component length to 64 bytes, and message length to 2,048 bytes. The client
uses only `INFO` and `ERROR`.

## Emitted Events

| Component | Level | Message event | Emission condition | Included metadata |
| --- | --- | --- | --- | --- |
| `client.runtime` | `INFO` | `runtime_started` | Client setup completed. | None. |
| `client.authentication` | `INFO` | `authentication_succeeded` | Initial authenticated heartbeat succeeded. | None. |
| `client.authentication` | `ERROR` | `authentication_failed` | Initial authenticated heartbeat failed. | None. |
| `client.attestation` | `INFO` | `attestation_submitted` | A newly observed agent submitted its endpoint attestation. | None. |
| `client.attestation` | `ERROR` | `attestation_failed` | Endpoint attestation submission failed. | None. |
| `client.heartbeat` | `ERROR` | `heartbeat_failed` | A periodic heartbeat failed. Successful periodic heartbeats are intentionally not logged. | None. |
| `client.command` | `INFO` | `command_received` | A command was received from the gateway. | None. |
| `client.command` | `INFO` | `command_completed` | The command result was accepted by the gateway. | None. |
| `client.command` | `ERROR` | `command_poll_failed` | Command polling failed. | None. |
| `client.command` | `ERROR` | `command_processing_failed` | Command processing failed before completion. | None. |
| `client.command` | `ERROR` | `command_result_submission_failed` | Result submission failed. | None. |
| `client.runtime` | `ERROR` | `runtime_loop_failed` | Either runtime loop exited with an error. | None. |

The client does not emit a record for every successful heartbeat because the
500-millisecond heartbeat interval would create an unbounded, low-value stream
of client-authored data. The authenticated heartbeat event remains available
at the gateway boundary.

The client cannot emit an event for failures that occur before mTLS setup has
completed, because no authenticated gateway transport exists at that point.
Those failures are returned to the invoking service manager without being
written to client storage.

## Prohibited Data

Client logs must not contain certificates, private keys, tokens, passwords,
environment variables, network addresses, full hostnames, browser data,
screenshots, file bytes, file paths, command payloads, terminal commands,
terminal output, raw errors, or request and response bodies. Error messages
are deliberately represented by fixed event names so that operating-system and
network details are not transmitted as diagnostic data.

## Stateless Command Replay Handling

The client retains verified command nonces only in process memory. A duplicate
command is rejected while the client process remains running. On restart, the
client has no local replay history by design. The gateway-generated signed
expiry window remains mandatory and limits the period in which a restarted
client can accept a previously issued command. The gateway remains responsible
for command issuance and must not reissue a consumed command.
