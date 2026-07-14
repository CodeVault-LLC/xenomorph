# File Explorer and Transfer Plan

## Purpose and Scope

This plan defines a gateway-mediated remote file workspace for internal support operations. The implemented workspace provides a responsive explorer, durable individual transfers, preconditioned mutations, explicit permanent deletion, capability-gated metadata updates, and bounded ZIP operations. The product does not provide managed trash or restore.

The feature is not a direct browser-to-agent file channel, a client trust mechanism, a malware-scanning service, or a means to bypass the gateway. The website and file API have no browser authentication, browser authorization, role check, credential, root grant, or per-agent root policy. The gateway remains the sole agent-identity, command-authenticity, audit, durable-operation, and command-dispatch boundary. The agent performs operations locally only after receiving a gateway-authored, integrity-protected, expiring command over the mutually authenticated channel. Every filesystem fact returned by the agent, including roots, paths, names, metadata, content hashes, capabilities, and operation outcomes, is client-authored evidence and must not be used as identity or authorization evidence.

The first implementation must target Windows, macOS, and Linux. It must provide an explicit capability model instead of presenting Unix semantics as universally available. Unsupported operations must be represented as unavailable capabilities, not emulated with lossy behavior.

## Current Implementation Status

Phase 0 command-authenticity controls, the implemented subset of Phase 1, Phase 2 individual transfers, Phase 3 safe mutations, and the Phase 4 implementation are present. The website discovers roots automatically, lists directories with cursor pagination, reads metadata, renders bounded previews, uploads and downloads individual files, and reconciles durable progress in a transfer drawer. It exposes create, move/rename, copy, duplicate, touch, truncate, append, single and bulk permanent deletion, dry-run review, explicit modified-time and POSIX-mode updates where available, and ZIP creation, bounded listing, and extraction. The gateway provides encrypted staged upload/download chunks, scoped leases, durable acknowledgements, resume/abort control, gateway-normalized operator paths, fixed signed archive limits, and preconditioned mutation dispatch. Deletion creates no gateway recovery record or agent-side staging copy.

Phase 1 retains its full cross-platform acceptance gate. Phase 4 code and Linux-native tests are present; its acceptance gate remains open until native Windows and macOS metadata/archive integration evidence is recorded. Windows currently reports metadata writes as unavailable instead of applying a path-based fallback. ZIP is the only enabled archive format; ZIP64, links, special entries, unsafe names, colliding paths, and over-limit archives are rejected. Phase 5 metrics, operational controls, chaos/load/recovery/accessibility evidence, and internal-user documentation remain absent. No release may describe the file workspace as complete until these residual gates pass.

## Existing-System Fit

The current shared schema has `FileChunk`, but it is only an uncoordinated chunk payload. It has no transfer manifest, durable resumption state, acknowledgement protocol, conflict contract, or retention control. It must not be extended incrementally into the file workspace protocol. The feature needs a dedicated gateway API, command vocabulary, gateway-owned durable state, and a versioned transfer protocol.

Component ownership is fixed as follows:

| Component | Owns | Does not own | Trust source |
| --- | --- | --- | --- |
| Website | Reactive display, internal-user intent collection, selection state, progress rendering, accessibility | Agent identity, browser authorization, direct agent transport, operation authenticity | Gateway API responses; browser input is website-authored and is not trust evidence |
| Gateway | Agent identity from mTLS, transfer/operation IDs, command issuance, audit trail, resumable-transfer coordination, retention | Browser authentication or authorization, local filesystem access, claims of client filesystem integrity, per-agent root policy | Verified mTLS client certificate; configured request-source label is audit-only |
| Agent | Automatic local root discovery, local enumeration, byte streaming, filesystem mutation, platform capability discovery, transfer checkpointing | Identity assertion, browser authorization decision, direct external publication | Gateway-authored and integrity-protected command received on mTLS channel |
| Object storage / transfer spool | Encrypted temporary and retained transfer bytes, manifests, lifecycle enforcement | Browser authentication, browser authorization, or filesystem interpretation | Gateway-issued short-lived, scoped credentials only |

The gateway must persist operation and transfer state before dispatching work. In-memory dashboard state, which is acceptable for the present terminal implementation, is not acceptable for transfers, audit records, resumptions, or idempotency keys.

## Product Surface

The implemented browser workspace has automatic root selection, a paged directory table, path navigation, a details/metadata inspector, bounded preview, and operation-status polling. It uses agent-reported capability flags as presentation hints. The planned workspace may add a mount-aware left navigation, virtualized directory grid/list, searchable command palette, transfer drawer, conflict-resolution dialog, and operation activity feed.

The explorer must use cursor pagination rather than an unbounded directory response. Each directory view returns a snapshot token, stable ordering definition, page cursor, estimated or exact count when affordable, and entries. The current UI requests pages on demand and cancels superseded list, preview, and metadata requests. The planned virtualized view renders only visible rows and preserves selection by opaque entry identity. It must never recursively enumerate a tree merely to calculate a folder size or populate a visible directory.

For future mutations, the UI creates an optimistic *pending* activity row but does not assert success until the gateway receives the authenticated client result. Directory refreshes consume a monotonic change sequence or invalidate the affected snapshot. Event streams are advisory; a reconnection must reconcile from the gateway read model using sequence cursors. Progress updates are coalesced and rate-limited so thousands of transfers do not cause browser rerender storms.

Required usability details include keyboard navigation, multi-select, range-select, copyable paths, clear empty/loading/error states, responsive layouts, screen-reader labels, focus restoration after dialogs, localization-safe date/size formatting, and explicit indication of whether an action is queued, running, paused, completed, failed, partially completed, cancelled, or awaiting a conflict choice. Drag-and-drop is not enabled in the current read-only explorer.

## Data and API Contracts

All externally visible requests and responses use typed, versioned structures with byte limits, enum allowlists, and forward-compatible optional fields. Paths are never accepted as trusted raw strings: the API takes an agent-reported `root_id` and a normalized relative path, while responses include a display path and opaque `entry_id`/snapshot identity. A request that names an entry should use its snapshot-bound entry identity where possible; the agent must still re-resolve it at execution time.

Define these gateway-managed resources and protocol records:

- `filesystem_root`: agent, agent-reported root ID, platform volume/mount identity, display label, supported read-only verbs, capability set, online state, and generation. It is client-authored presentation and routing data, not a grant.
- `directory_snapshot`: root ID, normalized relative path, sort/filter specification, issued time, expiry, opaque snapshot ID, and consistency/change marker.
- `file_entry`: opaque entry ID, display name, kind, size, timestamps, attributes, link state, content identity when available, and per-entry capabilities. All fields are client-authored observations.
- `operation`: gateway-generated ID, configured request-source label, agent ID, requested intent, idempotency key, state machine status, bounded result summary, timestamps, and audit correlation ID.
- `transfer`: transfer ID, direction, manifest version, source and destination binding, file set, conflict strategy, per-item state, byte/checksum checkpoints, retry budget, expiry, and retention deadline.

The implemented gateway API exposes automatic root discovery, cursor-paged directory listing, on-demand metadata, bounded content preview, transfer creation/status/control, and operation creation/status. Future phases may add search, cancellation, expanded conflict resolution, and a WebSocket or Server-Sent Events stream for gateway-generated state changes. API responses must distinguish `not_supported`, `not_found`, `stale_snapshot`, `conflict`, `offline`, `quota_exceeded`, `retryable_failure`, and `permanent_failure` without exposing sensitive local paths or credentials. `forbidden` is reserved for an operating-system or agent-side result, not browser access control.

The command protocol needs typed commands such as `files.roots.list`, `files.directory.list`, `files.metadata.get`, `files.preview.read`, `files.operation.execute`, `files.transfer.prepare`, `files.transfer.resume`, and `files.transfer.abort`. Each command includes a gateway-generated command ID, operation/transfer ID, expiry, nonce, requested root-relative operands, expected preconditions, and a cryptographic signature. The agent validates the signature, command type, expiry, replay nonce, byte limits, and all operands before execution. The file protocol version must change when its wire shape changes; version 2 replaces gateway-supplied root policies with automatic agent root discovery.

## Access and Path-Safety Model

The browser requires no credential, permission, role, or gateway grant to open the explorer for an agent that exists in the gateway directory. The website and dashboard API are deployment-restricted to the intended internal network population; that network boundary is the only browser access boundary. The gateway does not evaluate browser identity or authorize roots. A configured request-source label is written to audit records but is not an identity assertion or access decision.

The agent discovers its own roots whenever the explorer opens. Linux and macOS report the operating-system filesystem root (`/`); Windows reports every currently available logical drive. The agent resolves subsequent root IDs against its current root enumeration. A missing root ID is rejected as invalid protocol input, not denied by policy. The explorer is read-only. It can observe only data that the agent process itself can read; operating-system ACLs, mandatory access control, locks, quotas, immutable flags, and concurrent changes remain authoritative.

Path resolution must be race-resistant. The agent must start from an opened operating-system root handle and walk path components using platform-native no-follow semantics where available. It must reject traversal components, invalid encodings, NULs, alternate data stream syntax when disallowed, device namespaces, and resolution outside the selected root. A check-then-use path string sequence is not sufficient: attacker-controlled renames and symlink swaps can change the object between validation and mutation. Prefer handle/file-descriptor-relative operations; where a platform lacks them, reduce the operation to the safest available primitive, detect root escape before and after resolution, and report the residual platform limitation.

Symbolic links, junctions, mount points, bind mounts, hard links, reparse points, aliases, and shortcuts require explicit semantics. Directory enumeration must identify rather than silently follow links. Recursive actions default to no-follow. Cross-root and cross-volume moves must be treated as copy-then-verified-delete, never as an assumed atomic rename. Access checks are advisory UI information only; the local operation is authoritative and can still fail because ACLs, mandatory access control, locks, quotas, immutable flags, or concurrent changes changed after listing.

The gateway must enforce agent identity matching for command delivery and results, bounded queues, rate/concurrency limits, transfer quotas, and immutable append-only audit events. It must not add browser authentication, browser authorization, roles, per-agent root policies, or dual-control requirements to the read-only explorer. The audit record includes the configured request-source label, authenticated agent ID, request and result identifiers, root ID, redacted operands, byte/file counts, conflict choices, timestamps, and result classification. It must not log file bytes, secrets discovered in metadata, storage credentials, or full sensitive path names unless the configured retention rule explicitly permits them.

## Filesystem Operation Contract

Every mutation is an idempotent operation with a gateway-generated idempotency key, target preconditions, a dry-run/plan phase where meaningful, bounded execution, and a structured per-item result. Preconditions may include source entry identity, parent directory generation, expected file type, expected size/time, content hash, or absence/presence. If preconditions fail, the result is `conflict`; the system must not guess.

Support the following operation families in phased delivery:

| Family | Operations | Required safeguards |
| --- | --- | --- |
| Creation | empty file, directory, duplicate, touch, symbolic link | safe name validation; explicit link target semantics; no link traversal |
| Content | upload, download, overwrite, append, truncate, bounded preview | atomic staging where possible; expected-version checks; byte quotas; content integrity verification |
| Organization | rename, move, copy, bulk move/copy | same-root atomic rename when supported; collision strategy; recursion/cycle detection; cross-device fallback |
| Deletion | permanent deletion of files and bounded directory trees | explicit irreversible folder warning; two-step website dry-run review; source preconditions; no-follow traversal; depth and entry limits; filesystem-boundary control; top-level per-item result |
| Metadata | view/edit timestamps, POSIX mode, owner/group, ACL/attributes | capability-gated; least privilege; separate owner and ACL semantics; audit old/new values |
| Archive | create archive, list archive, extract archive | safe formats; archive-bomb limits; zip-slip prevention; no implicit execution or preview extraction |

The system must not pretend these operations mean the same thing on every filesystem. POSIX uid/gid/mode bits, Windows owners/DACL/SACL and attributes, macOS ACLs/resource forks, extended attributes, birth time, filesystem flags, alternate data streams, sparse allocation, case sensitivity, Unicode normalization, and timestamp resolution vary. Metadata responses need a common core plus namespaced platform fields and an `available`, `unavailable`, or `denied` state for every optional field. Metadata writes are explicit requested deltas, never a blanket replication of source metadata.

Name handling must preserve the original filesystem name as bytes/native representation where the platform permits while supplying a safe display representation. The UI must detect case-only rename hazards, reserved Windows names, path-length limits, normalization collisions, separator differences, and invalid source/destination names before dispatch; only the agent's final validation is authoritative. File identity cannot rely solely on a path: use platform object identifiers when available, otherwise retain a best-effort identity with a documented race caveat.

## Transfer Architecture

Transfers use a data plane distinct from command messages. Commands carry intent and signed execution constraints; file bytes move through a gateway-controlled encrypted transfer service or gateway-issued object-storage staging plane. The browser never receives agent credentials and the agent never receives unrestricted storage credentials.

For download (agent to browser), the gateway creates an export operation, the agent reads the selected source into chunks, and each chunk is uploaded using a short-lived capability scoped to one transfer, one object/key prefix, byte range, time window, and checksum scheme. The gateway verifies the manifest and serves completed export bytes through the dashboard API; it does not issue a browser credential or object-storage capability. For upload, the browser first stages bytes through the same gateway-controlled API, the gateway freezes a manifest, and the agent downloads only its scoped object after it receives the signed write operation. A future transfer phase must not introduce browser credentials, roles, per-agent root grants, or root policies without an explicit product decision.

Use a manifest containing canonical item IDs, root-relative paths, intended file type, sizes, chunk size, SHA-256 or stronger content digest, per-chunk checksums, transfer protocol version, and conflict/precondition strategy. Multipart/chunk upload acknowledgements are durable. Restart recovery derives the next missing chunk from stored acknowledgements; it never trusts a client-reported byte offset alone. Finalization verifies the full-object digest and expected size before atomically publishing the destination, then records the result.

Transfers must be bounded by configurable per-file, aggregate, directory-depth, item-count, queue-depth, bandwidth, and runtime limits. They use cancellation propagation, exponential backoff with jitter, resumable retries, bounded parallelism, adaptive chunk sizing, backpressure, and checksummed checkpoints. A lost connection leaves the transfer in `paused` or `recoverable` until expiry; reconnecting agents reconcile active transfer IDs from the gateway. Mid-transfer source mutations are detected through source version checks and final digest mismatch, producing a conflict rather than an ambiguous success.

Bulk transfer planning expands a selection into a manifest on the agent with maximum depth/count/bytes enforced during walk. It produces a preview with excluded, inaccessible, special, changed, and conflicting entries. The UI shows aggregate and per-item progress but does not wait for an exact pre-scan where a streaming plan is safer; it labels totals as estimated until final. Bulk operations have explicit all-or-nothing semantics only when the platform and destination staging support them. Otherwise results are per-item, resumable, and report partial completion clearly.

File bytes and sensitive metadata are encrypted in transit with TLS 1.3 and encrypted at rest in transfer staging. Storage object keys are unguessable and scoped; lifecycle rules delete unfinished staging data and completed exports at configured retention deadlines. Gateway storage credentials are never surfaced to browsers or agents. Malware scanning is explicitly out of scope: upload/download responses may carry a `not_scanned` classification and an extension hook for a future external scanning verdict, but must never claim clean or safe content without an integrated authoritative scanner.

## Failure, Conflict, and Recovery Semantics

All errors are structured, stable, and safe for internal users. The UI displays a usable remediation action while the audit/result record stores a machine-readable cause. Do not collapse permission denial, path disappearance, transfer corruption, and network loss into a generic failure.

| Condition | Required behavior |
| --- | --- |
| Permission denied or read-only filesystem | Mark the affected item denied; preserve other bulk-item results; offer no escalation path from the UI |
| Agent offline before dispatch | Keep the accepted operation queued only until its expiry; do not start a data-plane lease |
| Agent disconnects mid-transfer | Persist verified chunk state, revoke or expire leases, pause, and resume only after authenticated reconciliation |
| Browser disconnects | Continue gateway-dispatched background work; UI reconnects to gateway state rather than retaining authority in the tab |
| Source/destination changed | Fail precondition as conflict; present compare/retry/skip/rename decisions only when the operation supports them |
| Existing destination | Require an explicit, recorded conflict strategy: fail, skip, rename-new, replace, append, or apply-to-all within the requested batch |
| Checksum, size, or manifest mismatch | Quarantine/delete incomplete staging data, mark integrity failure, and never publish the destination atomically |
| Insufficient space/quota | Stop before irreversible publish where possible; retain resumable staging only within configured retention limits |
| Lock, sharing violation, ACL/MAC denial | Return platform-neutral class plus safe native detail; retry only classified transient cases |
| Cancellation | Cooperatively stop, clean temporary output, preserve verified resumable chunks when retention allows, and make cancellation idempotent |
| Gateway restart | Recover durable operations/transfers and replay no command unless its idempotency/precondition state permits it |
| Duplicate or delayed result | Deduplicate by operation/command/transfer IDs; accept only valid state transitions |

Deletion is an explicit permanent operation, not an operating-system recycle-bin integration. The website labels the action as irreversible, warns when selected folders include nested content, performs an agent-validated dry run, and requires a second destructive confirmation before execution. The agent revalidates source preconditions and builds a no-follow recursive plan limited to 10,000 entries and 64 directory levels. Unix rejects cross-device traversal and verifies device, inode, and entry type before each handle-relative removal. Windows uses its bounded path-based containment fallback and verifies file identity before removal. Links are removed without traversing their targets. Nested entries are not separate gateway mutation items, and a local failure or concurrent change can leave a partially removed tree because filesystem deletion is not transactional. No deleted content, hidden recovery path, retention payload, or restore mapping is created. Gateway operation and audit records remain mandatory and exclude relative paths and file content.

## Archive and Preview Safety

Archive creation and extraction are filesystem operations with resource controls. Supported formats must be allowlisted. Archive paths are normalized and rejected when absolute, traversing, link-escaping, device-like, or invalid for the destination platform. Enforce maximum entries, path depth, uncompressed bytes, compression ratio, nesting depth, CPU time, and temporary storage. Extraction does not preserve dangerous link/device entries by default and never executes archive contents.

Preview is a bounded read service, not an arbitrary file viewer. It reads at most a configured byte range, classifies binary/text conservatively, disables active content, escapes all displayed data, and does not render HTML, scripts, office macros, archives, or media decoders in privileged contexts. Large-file inspection supports sparse range reads, hex/text modes, and search only under explicit byte/time limits.

## Cross-Platform Capability Matrix

The workspace must implement an agent capability probe and report a versioned capability document. It includes operating system, filesystem type, automatically discovered roots, case sensitivity where detectable, max path behavior, atomic rename support, permanent-delete support, symlink/reparse support, ACL/owner/mode/timestamp fields, extended attributes, archive formats, sparse-file awareness, and available safe handle-relative primitives. This report is client-authored and informs presentation only; fixed protocol limits and the signed command boundary remain authoritative.

Platform adapters must isolate OS-specific code behind a narrow internal interface. Common code owns the normalized model, validation, operation state machine, transfer protocol, and tests. Platform code owns native file handles, metadata translation, error classification, root handles, and capability discovery. Tests must cover Windows and Unix compilation paths; integration tests run on Windows, macOS, and Linux filesystems, including case-sensitive and case-insensitive volumes where available.

## Security and Abuse-Resistance Requirements

- Require deployment-restricted browser access and mTLS-authenticated agents; derive agent identity only from the verified certificate at the gateway. Do not require browser credentials, roles, or permissions for the explorer.
- Verify cryptographic command signatures and replay protection in the agent before any filesystem action.
- Allowlist command types, operation verbs, archive formats, metadata fields, and conflict strategies. Discover roots from the agent; do not configure root allowlists or per-agent grants.
- Bound every body, manifest, page, path, metadata field, preview, search, queue, goroutine, upload, download, retry, and recursive walk.
- Use capability-scoped, short-lived data-plane credentials with least privilege for gateway and agent data-plane access; bind them to transfer, object prefix, direction, range, checksum, and expiry. Do not issue browser credentials.
- Never use a browser-provided agent ID, path, filename, size, hash, MIME type, or progress update as trust evidence.
- Treat filenames, metadata, MIME guesses, archive entries, and file content as untrusted for display and logging; defend against Unicode spoofing, control characters, formula injection in exports, and UI redress.
- Protect against symlink races, zip slip, archive bombs, decompression bombs, path traversal, overwrite races, TOCTOU, credential leakage, SSRF through remote filesystems, resource exhaustion, and confused-deputy writes.
- Do not execute, load, thumbnail, index, or invoke external handlers for transferred content in the agent or gateway.
- Record accepted operations and destructive actions in tamper-evident, retention-controlled audit storage; redact sensitive operands according to the configured retention rules.
- Threat-model each command, transfer state transition, data-plane lease, and platform adapter before implementation, mapping controls to OWASP ASVS Level 2 and NIST SSDF requirements in `.docs/code-quality.md`.

## Delivery Phases and Acceptance Gates

### Phase 0: Design and prerequisites

Produce the protocol specification, threat model, state diagrams, automatic-root-discovery model, retention model, audit schema, and capability matrix. Select durable storage and a gateway-controlled transfer storage implementation. Define the deployment network boundary and document that the explorer has no browser credentials, roles, permissions, root grants, or per-agent policies. Upgrade command authenticity to cryptographic verification. No filesystem feature is enabled in this phase.

Gate: security review approves the trust boundary, state transitions, path-resolution approach, storage credential scope, and abuse limits; tests prove forged, expired, replayed, and cross-agent commands are rejected.

### Phase 1: Read-only explorer

Implement automatic root discovery, cursor-paged listing, virtualized reactive UI, metadata inspection, bounded preview, snapshots, and capability reporting. Linux and macOS expose `/`; Windows exposes available logical drives. Do not require a policy file, exact agent-ID grant, credential, role, or browser permission. Retain no-follow enumeration, fixed request bounds, and signed command verification. Add directory search with bounded scope and cancellation.

Gate: large-directory benchmark meets the agreed interaction budget without unbounded memory or goroutines; all read endpoints operate without browser credentials or gateway root grants; command delivery and results remain bound to the mTLS-derived agent identity; race and cross-platform adapter tests pass.

### Phase 2: Durable individual transfers

Implement staged upload/download manifests, scoped data-plane leases, checksums, retries, pause/resume, transfer drawer, and durable transfer state. Support file-level conflict strategies and atomic destination publish. Do not add browser authentication, authorization, roles, root grants, or per-agent policies as a prerequisite.

Gate: network interruption, browser restart, agent restart, checksum mismatch, source mutation, quota exhaustion, and duplicate-result tests produce the documented state/result without data corruption.

### Phase 3: Safe mutations and permanent deletion

Implement create, rename, move, copy, duplicate, touch, truncate, append, and explicit permanent deletion. The destructive-operation product decision requires clear irreversible wording, a folder-content warning, a two-step website flow with an agent-validated dry run, source preconditions, bounded no-follow recursive deletion, and bulk top-level per-item result reporting. Managed trash and restore are not part of the product. The read-only explorer does not imply permission to mutate.

Gate: malicious path, symlink/reparse race, cross-volume move, collision, permission-denied, stale-snapshot, and cancellation tests pass on each supported OS.

### Phase 4: Metadata and archives

Status: implemented; cross-platform acceptance evidence pending.

Implement normalized metadata read/write, platform capability gating, archive creation/list/extraction, and all archive safety limits. Add UI explanations for unsupported and denied fields.

Gate: native metadata integration tests and archive traversal/bomb tests pass; no unavailable platform field is silently discarded or falsely reported as applied.

### Phase 5: Hardening and operations

Status: not implemented.

Add metrics, dashboards, alerts, retention enforcement, audit export controls, chaos testing, load testing, accessibility testing, and internal-user documentation. Set production defaults from observed capacity and security reviews.

Gate: all repository checks pass (`make fmt`, `make tidy`, `make test`, `make build`, `make lint`), plus race, cross-platform integration, security regression, performance, recovery, and accessibility suites. Release only after the documented threat model and operational limits are approved.

## Test Strategy and Performance Targets

Use deterministic fake filesystem and transfer adapters for unit tests; use isolated temporary roots for integration tests; never run destructive tests against developer home directories. Test malformed inputs, all state transitions, idempotency, cancellation, race windows, retention, automatic root discovery, no-browser-credential access, invalid Unicode/names, timestamp precision, metadata loss, special files, broken links, and partial bulk success. Fuzz path normalization, manifest decoding, archive parsing, and pagination cursors. Run `go test -race` and platform-specific integration suites in CI.

Set measurable service-level objectives before implementation. Suggested initial budgets are: first rendered directory page under 200 ms after gateway response; visible-row interaction at 60 fps; no full-directory allocation for browsing; bounded event updates per transfer; and transfer throughput limited by fixed service limits rather than UI work. Capacity tests must include directories with millions of entries, deeply nested trees at the protocol maximum, thousands of concurrent transfer records, large sparse files, high-latency networks, and reconnect storms. The implementation may tune exact limits only through configuration reviewed with operations and security.

## Explicit Non-Goals

The feature explores roots that the agent process can access and performs only explicitly signed, allowlisted, bounded operations. It does not bypass operating-system permissions, provide arbitrary shell execution, provide direct peer-to-peer transfer, provide automatic malware verdicts, act as a consumer cloud-drive synchronization client, index every file in the background, silently overwrite data, promise metadata equivalence across filesystems, provide managed trash or restore, or provide unbounded or transactional recursive deletion. Remote filesystem mounts, external-share connectors, deduplication across agents, content scanning, and persistent content indexing require a separate product decision and threat-model update.
