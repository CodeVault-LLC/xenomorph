# File Explorer and Transfer Plan

## Purpose and Scope

This plan defines a gateway-mediated remote file workspace for authorized internal support operations. The workspace provides a responsive explorer, individual and bulk transfers, safe filesystem mutations, metadata inspection and editing, archive operations, and recovery from a managed trash area.

The feature is not a direct browser-to-agent file channel, a client trust mechanism, a malware-scanning service, or a means to bypass the gateway. The gateway remains the sole authentication, authorization, policy, audit, and command-dispatch boundary. The agent performs operations locally only after receiving a gateway-authored, integrity-protected, expiring command over the mutually authenticated channel. Every filesystem fact returned by the agent, including paths, names, metadata, content hashes, capabilities, and operation outcomes, is client-authored evidence and must not be used as identity or authorization evidence.

The first implementation must target Windows, macOS, and Linux. It must provide an explicit capability model instead of presenting Unix semantics as universally available. Unsupported operations must be represented as unavailable capabilities, not emulated with lossy behavior.

## Existing-System Fit

The current shared schema has `FileChunk`, but it is only an uncoordinated chunk payload. It has no transfer manifest, authenticated operator intent, durable resumption state, path policy, acknowledgement protocol, conflict contract, or retention control. It must not be extended incrementally into the file workspace protocol. The feature needs a dedicated gateway API, command vocabulary, gateway-owned durable state, and a versioned transfer protocol.

Component ownership is fixed as follows:

| Component | Owns | Does not own | Trust source |
| --- | --- | --- | --- |
| Website | Reactive display, operator intent collection, selection state, progress rendering, accessibility | Agent identity, authorization, direct agent transport, operation authenticity | Gateway API responses; browser input is operator-authored |
| Gateway | Operator authentication and authorization, agent identity from mTLS, path-root policy, transfer/operation IDs, command issuance, audit trail, resumable-transfer coordination, retention | Local filesystem access or claims of client filesystem integrity | Verified mTLS client certificate and authenticated operator session |
| Agent | Local enumeration, byte streaming, filesystem mutation, platform capability discovery, transfer checkpointing | Identity assertion, policy decision, direct external publication | Gateway-authored and integrity-protected command received on mTLS channel |
| Object storage / transfer spool | Encrypted temporary and retained transfer bytes, manifests, lifecycle enforcement | Authorization or filesystem interpretation | Gateway-issued short-lived, scoped credentials only |

The gateway must persist operation and transfer state before dispatching work. In-memory dashboard state, which is acceptable for the present terminal implementation, is not acceptable for transfers, trash recovery, audit records, resumptions, or idempotency keys.

## Product Surface

The browser workspace has a mount-aware left navigation, a virtualized directory grid/list, a path breadcrumb, a searchable command palette, a transfer drawer, a details/metadata inspector, a conflict-resolution dialog, and an operation activity feed. It uses server-driven capability flags to enable actions only when the selected entries and their filesystem support them.

The explorer must use cursor pagination rather than an unbounded directory response. Each directory view returns a snapshot token, stable ordering definition, page cursor, estimated or exact count when affordable, and entries. The UI renders only visible rows (DOM virtualization), requests pages on demand, preserves selection by opaque entry identity, and cancels superseded list, preview, and metadata requests. It must never recursively enumerate a tree merely to calculate a folder size or populate a visible directory.

The UI is reactive: a mutation creates an optimistic *pending* activity row, but it does not assert success until the gateway receives the authenticated client result. Directory refreshes consume a monotonic change sequence or invalidate the affected snapshot. Event streams are advisory; a reconnection must reconcile from the gateway read model using sequence cursors. Progress updates are coalesced and rate-limited so thousands of transfers do not cause browser rerender storms.

Required usability details include keyboard navigation, multi-select, range-select, drag-and-drop only within authorized roots, copyable paths, clear empty/loading/error states, responsive layouts, screen-reader labels, focus restoration after dialogs, localization-safe date/size formatting, and explicit indication of whether an action is queued, running, paused, completed, failed, partially completed, cancelled, or awaiting a conflict choice.

## Data and API Contracts

All externally visible requests and responses use typed, versioned structures with byte limits, enum allowlists, and forward-compatible optional fields. Paths are never accepted as trusted raw strings: the API takes a gateway-issued `root_id` and a normalized relative path, while responses include a display path and opaque `entry_id`/snapshot identity. A request that names an entry should use its snapshot-bound entry identity where possible; the agent must still re-resolve it at execution time.

Define these gateway-owned resources:

- `filesystem_root`: agent, root ID, platform volume/mount identity, display label, policy, capability set, online state, and generation.
- `directory_snapshot`: root ID, normalized relative path, sort/filter specification, issued time, expiry, opaque snapshot ID, and consistency/change marker.
- `file_entry`: opaque entry ID, display name, kind, size, timestamps, attributes, link state, content identity when available, and per-entry capabilities. All fields are client-authored observations.
- `operation`: gateway-generated ID, operator ID, agent ID, requested intent, authorization decision, idempotency key, state machine status, bounded result summary, timestamps, policy version, and audit correlation ID.
- `transfer`: transfer ID, direction, manifest version, source and destination binding, file set, conflict policy, per-item state, byte/checksum checkpoints, retry budget, expiry, and retention deadline.
- `trash_item`: gateway-owned logical record mapping a recoverable item to its original root/path, deletion operation, available restore target, retention deadline, and integrity metadata.

The gateway API should expose root listing; cursor-paged directory listing; on-demand metadata; bounded content preview; search; operation creation/status/cancellation; transfer creation/status/control; conflict resolution; trash listing/restore/purge; and a WebSocket or Server-Sent Events stream for gateway-generated state changes. API responses must distinguish `forbidden`, `not_supported`, `not_found`, `stale_snapshot`, `conflict`, `offline`, `quota_exceeded`, `retryable_failure`, and `permanent_failure` without exposing sensitive local paths or credentials.

The command protocol needs typed commands such as `files.roots.list`, `files.directory.list`, `files.metadata.get`, `files.preview.read`, `files.operation.execute`, `files.transfer.prepare`, `files.transfer.resume`, and `files.transfer.abort`. Each command includes a gateway-generated command ID, operation/transfer ID, policy version, expiry, nonce, requested root-relative operands, expected preconditions, and a cryptographic signature. The agent validates the signature, command type, expiry, replay nonce, byte limits, and all operands before execution. The existing non-cryptographic signature-presence check is insufficient for this feature and must be replaced by verified command authenticity before any filesystem command is enabled.

## Authorization and Path-Safety Model

Authorization is evaluated at the gateway before command dispatch and rechecked at the agent against the signed policy claim. Policies grant verbs on named roots, not arbitrary absolute paths. A policy can constrain maximum recursive depth, file count, aggregate bytes, individual file size, extensions/MIME classes, archive extraction limits, whether destructive actions require a second approval, and whether downloads/uploads may leave the service storage boundary.

Path resolution must be race-resistant. The agent must start from an opened, policy-authorized root handle and walk path components using platform-native no-follow semantics where available. It must reject traversal components, invalid encodings, NULs, alternate data stream syntax when disallowed, device namespaces, and resolution outside the root. A check-then-use path string sequence is not sufficient: attacker-controlled renames and symlink swaps can change the object between validation and mutation. Prefer handle/file-descriptor-relative operations; where a platform lacks them, reduce the operation to the safest available primitive, detect root escape before and after resolution, and report the residual platform limitation.

Symbolic links, junctions, mount points, bind mounts, hard links, reparse points, aliases, and shortcuts require explicit semantics. Directory enumeration must identify rather than silently follow links. Recursive actions default to no-follow. Cross-root and cross-volume moves must be treated as copy-then-verified-delete, never as an assumed atomic rename. Access checks are advisory UI information only; the local operation is authoritative and can still fail because ACLs, mandatory access control, locks, quotas, immutable flags, or concurrent changes changed after listing.

The gateway must enforce role-based and agent-scoped authorization, optional dual control for destructive recursive actions, fresh authentication/re-authentication for high-impact actions, rate/concurrency limits, per-operator transfer quotas, and immutable append-only audit events. The audit record includes operator identity, authenticated agent ID, request and result identifiers, policy decision, root ID, redacted operands, byte/file counts, conflict choices, timestamps, and result classification. It must not log file bytes, secrets discovered in metadata, storage credentials, or full sensitive path names unless the authorized retention policy explicitly permits them.

## Filesystem Operation Contract

Every mutation is an idempotent operation with a gateway-generated idempotency key, target preconditions, a dry-run/plan phase where meaningful, bounded execution, and a structured per-item result. Preconditions may include source entry identity, parent directory generation, expected file type, expected size/time, content hash, or absence/presence. If preconditions fail, the result is `conflict`; the system must not guess.

Support the following operation families in phased delivery:

| Family | Operations | Required safeguards |
| --- | --- | --- |
| Creation | empty file, directory, duplicate, touch, symbolic link | authorized parent; safe name validation; explicit link target semantics; no link traversal |
| Content | upload, download, overwrite, append, truncate, bounded preview | atomic staging where possible; expected-version checks; byte quotas; content integrity verification |
| Organization | rename, move, copy, bulk move/copy | same-root atomic rename when supported; collision policy; recursion/cycle detection; cross-device fallback |
| Deletion and recovery | move to managed trash, restore, permanent purge | no silent permanent deletion; restore conflict policy; retention and secure cleanup policy |
| Metadata | view/edit timestamps, POSIX mode, owner/group, ACL/attributes | capability-gated; least privilege; separate owner and ACL semantics; audit old/new values |
| Archive | create archive, list archive, extract archive | safe formats; archive-bomb limits; zip-slip prevention; no implicit execution or preview extraction |

The system must not pretend these operations mean the same thing on every filesystem. POSIX uid/gid/mode bits, Windows owners/DACL/SACL and attributes, macOS ACLs/resource forks, extended attributes, birth time, filesystem flags, alternate data streams, sparse allocation, case sensitivity, Unicode normalization, and timestamp resolution vary. Metadata responses need a common core plus namespaced platform fields and an `available`, `unavailable`, or `denied` state for every optional field. Metadata writes are explicit requested deltas, never a blanket replication of source metadata.

Name handling must preserve the original filesystem name as bytes/native representation where the platform permits while supplying a safe display representation. The UI must detect case-only rename hazards, reserved Windows names, path-length limits, normalization collisions, separator differences, and invalid source/destination names before dispatch; only the agent's final validation is authoritative. File identity cannot rely solely on a path: use platform object identifiers when available, otherwise retain a best-effort identity with a documented race caveat.

## Transfer Architecture

Transfers use a data plane distinct from command messages. Commands carry intent and authorization; file bytes move through a gateway-controlled encrypted transfer service or gateway-authorized object-storage staging plane. The browser never receives agent credentials and the agent never receives unrestricted storage credentials.

For download (agent to operator), the gateway authorizes an export, the agent reads an authorized source into chunks, and each chunk is uploaded using a short-lived capability scoped to one transfer, one object/key prefix, byte range, time window, and checksum scheme. The gateway verifies the manifest and publishes a separate short-lived operator download capability only after completion or for explicitly authorized streaming. For upload, the browser first stages bytes into the same controlled plane, the gateway freezes a manifest, and the agent downloads only its scoped object after it receives the signed write operation.

Use a manifest containing canonical item IDs, root-relative paths, intended file type, sizes, chunk size, SHA-256 or stronger content digest, per-chunk checksums, transfer protocol version, and conflict/precondition policy. Multipart/chunk upload acknowledgements are durable. Restart recovery derives the next missing chunk from stored acknowledgements; it never trusts a client-reported byte offset alone. Finalization verifies the full-object digest and expected size before atomically publishing the destination, then records the result.

Transfers must be bounded by configurable per-file, aggregate, directory-depth, item-count, queue-depth, bandwidth, and runtime limits. They use cancellation propagation, exponential backoff with jitter, resumable retries, bounded parallelism, adaptive chunk sizing, backpressure, and checksummed checkpoints. A lost connection leaves the transfer in `paused` or `recoverable` until expiry; reconnecting agents reconcile active transfer IDs from the gateway. Mid-transfer source mutations are detected through source version checks and final digest mismatch, producing a conflict rather than an ambiguous success.

Bulk transfer planning expands a selection into a manifest on the agent with maximum depth/count/bytes enforced during walk. It produces a preview with excluded, inaccessible, special, changed, and conflicting entries. The UI shows aggregate and per-item progress but does not wait for an exact pre-scan where a streaming plan is safer; it labels totals as estimated until final. Bulk operations have explicit all-or-nothing semantics only when the platform and destination staging support them. Otherwise results are per-item, resumable, and report partial completion clearly.

File bytes and sensitive metadata are encrypted in transit with TLS 1.3 and encrypted at rest in staging. Storage object keys are unguessable and scoped; lifecycle rules delete unfinished staging data, completed exports, and trash payloads at policy-defined deadlines. Gateway storage credentials are never surfaced to operators or agents. Malware scanning is explicitly out of scope: upload/download responses may carry a `not_scanned` classification and a policy hook for a future external scanning verdict, but must never claim clean or safe content without an integrated authoritative scanner.

## Failure, Conflict, and Recovery Semantics

All errors are structured, stable, and safe for operators. The UI displays a usable remediation action while the audit/result record stores a machine-readable cause. Do not collapse permission denial, path disappearance, transfer corruption, and network loss into a generic failure.

| Condition | Required behavior |
| --- | --- |
| Permission denied or read-only filesystem | Mark the affected item denied; preserve other bulk-item results; offer no escalation path from the UI |
| Agent offline before dispatch | Keep the approved operation queued only until its expiry; do not start a data-plane lease |
| Agent disconnects mid-transfer | Persist verified chunk state, revoke or expire leases, pause, and resume only after authenticated reconciliation |
| Browser disconnects | Continue gateway-authorized background work; UI reconnects to gateway state rather than retaining authority in the tab |
| Source/destination changed | Fail precondition as conflict; present compare/retry/skip/rename decisions only when policy permits |
| Existing destination | Require an explicit, recorded conflict strategy: fail, skip, rename-new, replace, append, or apply-to-all within the authorized batch |
| Checksum, size, or manifest mismatch | Quarantine/delete incomplete staging data, mark integrity failure, and never publish the destination atomically |
| Insufficient space/quota | Stop before irreversible publish where possible; retain resumable staging only within retention policy |
| Lock, sharing violation, ACL/MAC denial | Return platform-neutral class plus safe native detail; retry only classified transient cases |
| Cancellation | Cooperatively stop, clean temporary output, preserve verified resumable chunks when policy allows, and make cancellation idempotent |
| Gateway restart | Recover durable operations/transfers and replay no command unless its idempotency/precondition state permits it |
| Duplicate or delayed result | Deduplicate by operation/command/transfer IDs; accept only valid state transitions |

Trash is a managed feature, not an assumption that the operating system recycle bin is available. Default deletion moves an eligible entry into a configured, root-bound managed trash namespace by atomic rename where possible. The gateway stores original location and retention metadata. If a root cannot support managed trash safely, deletion is unavailable unless a separately authorized permanent-purge policy exists. Restore always checks destination preconditions and uses the same explicit collision policy. Purge is delayed, auditable, retention-governed, and prevents recovery only after successful deletion confirmation.

## Archive and Preview Safety

Archive creation and extraction are filesystem operations with resource controls. Supported formats must be allowlisted. Archive paths are normalized and rejected when absolute, traversing, link-escaping, device-like, or invalid for the destination platform. Enforce maximum entries, path depth, uncompressed bytes, compression ratio, nesting depth, CPU time, and temporary storage. Extraction does not preserve dangerous link/device entries by default and never executes archive contents.

Preview is a bounded read service, not an arbitrary file viewer. It reads at most a configured byte range, classifies binary/text conservatively, disables active content, escapes all displayed data, and does not render HTML, scripts, office macros, archives, or media decoders in privileged contexts. Large-file inspection supports sparse range reads, hex/text modes, and search only under explicit byte/time limits.

## Cross-Platform Capability Matrix

Before enabling the workspace, implement an agent capability probe and report a versioned capability document. It includes operating system, filesystem type, root policy, case sensitivity where detectable, max path behavior, atomic rename support, managed trash support, symlink/reparse support, ACL/owner/mode/timestamp fields, extended attributes, archive formats, sparse-file awareness, and available safe handle-relative primitives. This report is client-authored and informs presentation only; the gateway policy remains authoritative.

Platform adapters must isolate OS-specific code behind a narrow internal interface. Common code owns the normalized model, validation, operation state machine, transfer protocol, and tests. Platform code owns native file handles, metadata translation, error classification, root handles, and capability discovery. Tests must cover Windows and Unix compilation paths; integration tests run on Windows, macOS, and Linux filesystems, including case-sensitive and case-insensitive volumes where available.

## Security and Abuse-Resistance Requirements

- Require authenticated operators and mTLS-authenticated agents; derive agent identity only from the verified certificate at the gateway.
- Verify cryptographic command signatures and replay protection in the agent before any filesystem action.
- Allowlist command types, operation verbs, archive formats, roots, metadata fields, and conflict strategies.
- Bound every body, manifest, page, path, metadata field, preview, search, queue, goroutine, upload, download, retry, and recursive walk.
- Use capability-scoped, short-lived data-plane credentials with least privilege; bind them to transfer, object prefix, direction, range, checksum, and expiry.
- Never use a browser-provided agent ID, path, filename, size, hash, MIME type, or progress update as trust evidence.
- Treat filenames, metadata, MIME guesses, archive entries, and file content as untrusted for display and logging; defend against Unicode spoofing, control characters, formula injection in exports, and UI redress.
- Protect against symlink races, zip slip, archive bombs, decompression bombs, path traversal, overwrite races, TOCTOU, credential leakage, SSRF through remote filesystems, resource exhaustion, and confused-deputy writes.
- Do not execute, load, thumbnail, index, or invoke external handlers for transferred content in the agent or gateway.
- Record authorization and destructive actions in tamper-evident, retention-controlled audit storage; redact sensitive operands according to policy.
- Threat-model each command, transfer state transition, data-plane lease, and platform adapter before implementation, mapping controls to OWASP ASVS Level 2 and NIST SSDF requirements in `.docs/code-quality.md`.

## Delivery Phases and Acceptance Gates

### Phase 0: Design and prerequisites

Produce the protocol specification, threat model, state diagrams, root-policy model, retention policy, audit schema, and capability matrix. Select durable storage and a gateway-controlled transfer storage implementation. Define operator roles and approval requirements. Upgrade command authenticity to cryptographic verification. No filesystem feature is enabled in this phase.

Gate: security review approves the trust boundary, state transitions, path-resolution approach, storage credential scope, and abuse limits; tests prove forged, expired, replayed, and cross-agent commands are rejected.

### Phase 1: Read-only explorer

Implement root discovery, cursor-paged listing, virtualized reactive UI, metadata inspection, bounded preview, snapshots, and capability reporting. Limit browsing to explicitly authorized roots and no-follow enumeration. Add directory search with bounded scope and cancellation.

Gate: large-directory benchmark meets the agreed interaction budget without unbounded memory or goroutines; all read endpoints enforce operator/agent scope; race and cross-platform adapter tests pass.

### Phase 2: Durable individual transfers

Implement staged upload/download manifests, scoped data-plane leases, checksums, retries, pause/resume, transfer drawer, and durable transfer state. Support file-level conflict strategies and atomic destination publish.

Gate: network interruption, browser restart, agent restart, checksum mismatch, source mutation, quota exhaustion, and duplicate-result tests produce the documented state/result without data corruption.

### Phase 3: Safe mutations and trash

Implement create, rename, move, copy, duplicate, touch, truncate, append, managed trash, restore, and permitted purge. Add dry-run plans, preconditions, bulk per-item result reporting, and destructive-action authorization.

Gate: malicious path, symlink/reparse race, cross-volume move, collision, permission-denied, stale-snapshot, and cancellation tests pass on each supported OS.

### Phase 4: Metadata and archives

Implement normalized metadata read/write, platform capability gating, archive creation/list/extraction, and all archive safety limits. Add UI explanations for unsupported and denied fields.

Gate: native metadata integration tests and archive traversal/bomb tests pass; no unavailable platform field is silently discarded or falsely reported as applied.

### Phase 5: Hardening and operations

Add metrics, dashboards, alerts, retention enforcement, audit export controls, chaos testing, load testing, accessibility testing, and operator documentation. Set production defaults from observed capacity and security reviews.

Gate: all repository checks pass (`make fmt`, `make tidy`, `make test`, `make build`, `make lint`), plus race, cross-platform integration, security regression, performance, recovery, and accessibility suites. Release only after the documented threat model and operational limits are approved.

## Test Strategy and Performance Targets

Use deterministic fake filesystem and transfer adapters for unit tests; use isolated temporary roots for integration tests; never run destructive tests against developer home directories. Test malformed inputs, all state transitions, idempotency, cancellation, race windows, retention, authorization, invalid Unicode/names, timestamp precision, metadata loss, special files, broken links, and partial bulk success. Fuzz path normalization, manifest decoding, archive parsing, and pagination cursors. Run `go test -race` and platform-specific integration suites in CI.

Set measurable service-level objectives before implementation. Suggested initial budgets are: first rendered directory page under 200 ms after gateway response; visible-row interaction at 60 fps; no full-directory allocation for browsing; bounded event updates per transfer; and transfer throughput limited by configured policy rather than UI work. Capacity tests must include directories with millions of entries, deeply nested trees at the policy maximum, thousands of concurrent transfer records, large sparse files, high-latency networks, and reconnect storms. The implementation may tune exact limits only through configuration reviewed with operations and security.

## Explicit Non-Goals

The feature does not provide unrestricted disk access, arbitrary shell execution, direct peer-to-peer transfer, automatic malware verdicts, a consumer cloud-drive synchronization client, background indexing of every file, silent destructive overwrite, or a promise of metadata equivalence across filesystems. Any future extension that introduces remote filesystem mounts, external-share connectors, deduplication across agents, content scanning, or persistent content indexing requires a separate threat model and plan.
