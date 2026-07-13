# Terminal Capability and Workspace Plan

## Purpose and Scope

This plan defines the next terminal investment for the internal remote screening platform. The immediate objective is a capability-aware remote command workspace: an operator can select only a shell that the connected agent has reported as available, receive completion suggestions drawn from that agent's executable search path, submit bounded commands, and understand the command lifecycle without interpreting a generic subprocess error.

The current feature is not an interactive terminal. It dispatches one bounded shell command at a time, retains an in-memory working-directory state, and returns bounded combined output after the process exits. The product must describe it as a **remote command workspace** until an explicitly designed PTY mode is delivered. The name may remain “Terminal” in navigation for continuity, but the UI must not imply support for full-screen programs, job control, shell completion, resize events, or byte-stream terminal semantics that do not exist.

This work owns terminal capability discovery, shell selection, command completion, command lifecycle, session ergonomics, and the gateway read model needed for those features. It does not create a browser-to-agent channel, make client claims authoritative, grant browser permissions, install software on agents, enumerate arbitrary filesystem content, or bypass the gateway for command issuance or results.

## Current-System Findings

The client supports `bash`, `zsh`, `sh`, `powershell`, `pwsh`, and `cmd`, but the website presents choices from an operating-system heuristic. On Linux it offers `zsh` even when `zsh` is absent. The client subsequently calls `exec.CommandContext(..., "zsh", ...)`, which produces the observed `executable file not found in $PATH` failure. The client already performs limited local discovery for its default shell through `$SHELL`, `/bin/bash`, and `exec.LookPath`, but that result is not reported to the gateway or browser.

The gateway's shell normalization is also permissive: unknown values fall back to `bash`, while the client falls back to `sh`. A request can therefore be displayed as one shell and execute as another. The session is created before the client confirms the selected shell. The gateway stores at most 12 sessions and 300 entries per agent in process memory, polls entry state from the browser, combines stdout and stderr, has no cancellation path, and loses all terminal state on restart.

The present `cd` implementation parses whitespace-delimited arguments and updates an emulated working directory. It does not have the quoting, expansion, or compound-command semantics of the selected shell. This is a material reason to distinguish the current command workspace from an interactive terminal.

## Ownership and Trust Model

| Component     | Owns                                                                                                                                                                                  | Does not own                                                                                                   | Trust source                                                                     |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Website       | Capability presentation, local completion filtering, operator command text, keyboard behavior, and rendering lifecycle state                                                          | Agent identity, capability truth, authorization, direct execution, or result authenticity                      | Gateway API responses; browser input is operator-authored and not trust evidence |
| Gateway       | Authenticated agent routing, server-generated IDs, validation of bounded API shapes, session/command lifecycle, capability cache, dispatch policy, audit events, and result ingestion | Local executable discovery, direct process execution, or treating capability reports as authorization evidence | mTLS-authenticated agent identity and gateway-owned state                        |
| Client        | Local shell and PATH discovery, bounded command-catalog construction, shell availability revalidation, command execution, and structured result facts                                 | Agent identity assertions, browser authorization, command authenticity, or direct publication to the browser   | Gateway-authored, integrity-protected, unexpired command envelopes               |
| Shared schema | Versioned heartbeat and command-result data shapes                                                                                                                                    | Dashboard policy, browser session state, or local execution behavior                                           | Generated artifacts from the reviewed protobuf source                            |

`agent_id`, capability-record ID, capability generation, timestamps assigned by the gateway, terminal session IDs, command IDs, command queue status, audit IDs, and policy decisions are server-authored. Requested shell, command text, user-entered working-directory intent, labels, and completion selection are operator-authored. Shell availability, executable names, default shell, command catalog entries, current directory, stdout, stderr, exit code, and process facts are client-authored. Client-authored capability data is useful presentation and routing evidence only; it must never establish identity, authorization, or host integrity.

## Product Direction

### Capability-aware shell selection

Replace every operating-system-derived shell choice in the website and gateway with an agent-reported capability record. An available shell is an explicitly normalized shell identifier whose executable was resolved locally by the client at the time of discovery. The selector must show only `available` shells by default, mark unavailable or stale information distinctly when it is deliberately exposed for diagnosis, and choose the reported default shell for a new session.

The capability record must include a schema version, client-observed time, bounded TTL, normalized default shell, and a bounded list of supported shell identifiers with availability state. It must not send raw executable paths, `$PATH`, shell startup files, environment variables, aliases, or arbitrary command output. The initial values are `available`, `unavailable`, and `unknown`; the browser must render `unknown` as unavailable until fresh data arrives.

The client must resolve the actual binary used by execution with `exec.LookPath` or the platform equivalent, including `powershell.exe`, `pwsh`, `cmd.exe`, `bash`, `zsh`, and `sh`. It must distinguish a missing executable from an unsupported platform. It must revalidate the selected executable immediately before execution because packages and PATH can change after the heartbeat. A stale or missing shell must produce a structured `shell_unavailable` result containing the normalized requested shell and the fresh available-shell list; it must not be flattened into a generic exit-code-1 error.

The gateway must validate a requested session shell against the most recent non-expired capability record for the selected authenticated agent. It must reject an unsupported, unknown, or stale requested shell with a typed conflict response. The client remains the final executor and reports revalidation failures. Unknown shell values must be rejected at every boundary; they must not silently normalize to `bash` or `sh`.

### Command completion based on local reality

Command completion must be agent-specific and shell-aware. The initial command catalog contains only:

- normalized built-ins for each supported shell, maintained as reviewed static metadata;
- executable basenames discovered by the agent in its current executable search path;
- explicit workspace actions such as the supported working-directory control, when those are not shell commands.

The client builds the executable portion from executable files in the directories it would use for process lookup. It must canonicalize and deduplicate by basename, enforce a strict maximum entry count and byte budget, skip unreadable directories and non-executable files, and refresh only on a bounded cadence or an explicit signed discovery request. It must not recursively scan the filesystem, enumerate installed packages, read shell startup files, invoke a shell merely to ask for completions, or expose aliases, functions, environment variables, absolute paths, or arbitrary file names in the first release.

The gateway stores the latest bounded catalog as client-authored data associated with an authenticated agent and capability generation. The dashboard fetches it only through a gateway endpoint. The browser filters locally after the initial gateway response; it must not emit a network request per keystroke. Suggestions are prefix-first, then case-insensitive subsequence matches, limited to a small fixed result count, keyboard navigable, and inserted only into the first command token in phase one. Each suggestion must identify whether it is a shell builtin or an executable. The UI must never claim that a listed command is safe, permitted, or guaranteed to succeed.

Later completion phases may add arguments, paths, package managers, aliases, shell functions, and context-sensitive completion only after a separate privacy and execution-safety design. Those features can reveal filenames, configuration, credentials, or shell initialization side effects and are out of scope for the initial catalog.

### Command composer and session experience

Retain the multiline composer, visible Run command action, Enter-to-run, and Shift+Enter-to-insert behavior. Add a command-completion popover that does not obscure the Run action, follows focus correctly, supports Escape dismissal, arrow-key selection, Tab acceptance, pointer selection, and screen-reader announcement. It must be disabled while the agent is offline, capabilities are absent, or a command is being submitted.

Show the selected shell and working directory as compact session metadata adjacent to the composer. The directory is client-authored and must be labeled as last confirmed by the agent. A new session must use the gateway's validated capability default, not a browser OS heuristic. The new-session menu must not offer a shell that the capability record does not mark available.

Add session rename, duplicate command into composer, copy command/output separately, clear local visible output without deleting audit history, pin/unpin, and searchable history as planned enhancements. History search must be gateway-backed and scope results to one authenticated agent and session by default. History entries must retain the original operator-authored command, result classification, timestamps, shell, and confirmed directory. The UI must render remote output as text, never HTML.

Replace generic failure strings with a stable result classification and a concise remediation surface. Required categories include `queued`, `dispatched`, `running`, `completed`, `nonzero_exit`, `timed_out`, `cancelled`, `shell_unavailable`, `working_directory_unavailable`, `agent_offline`, `queue_unavailable`, `rejected_by_policy`, `output_truncated`, and `result_lost`. The operator-facing message may include bounded client detail, while the machine-readable category remains stable. A shell-unavailable result should offer a new-session action preselected to an available shell when current capability data exists; it must not auto-switch the existing session.

### Execution and state semantics

Phase one must formalize the current mode as sequential, non-interactive command execution. The gateway must permit at most one active command per terminal session and preserve command order. It must provide idempotency protection for a submit retry, queue-depth limits, expiry, dispatch timestamps, and a final state for every accepted command. The browser must not optimistically display success before an authenticated result arrives.

Replace the whitespace-based `cd` parser with an explicit, typed “set working directory” session action, validated and executed by the client, with a structured confirmed-directory result. Do not infer shell quoting semantics in Go. For compatibility, a plain `cd` command can remain a documented deprecated convenience only if its limitations are explicit; it must not be represented as equivalent to a native shell. Compound shell commands continue to execute through the selected shell but must not mutate the gateway session directory unless the typed action succeeds.

Execution results must carry separate bounded stdout and stderr, exit status, outcome classification, started/completed timestamps, confirmed shell, confirmed directory, timeout/truncation flags, and a safe bounded diagnostic code. The gateway must preserve the server-authored lifecycle and attach the authenticated client result without making client values authoritative. The result API must support cursor pagination rather than returning an unbounded session history.

### Interactive terminal mode: separate future product

A true interactive terminal requires a PTY on Unix and ConPTY on Windows, a framed bidirectional gateway stream, terminal resize, cancellation and signal semantics, ordered byte frames, backpressure, reconnect behavior, retention limits, and audited session lifetime. It must be designed as a new protocol and feature flag, not added to the present command result endpoint. Browser access remains gateway-mediated; a browser WebSocket terminates at the gateway, which applies agent and session binding before relaying gateway-authorized frames over the authenticated agent channel. Direct browser-to-agent sockets are prohibited.

The PTY phase must specify terminal escape handling, binary framing, frame/output size limits, detach and reconnect behavior, idle expiry, concurrent-view policy, process-tree termination, sensitive-output handling, and the operational risks of interactive tools. Completion for a PTY session belongs to the shell itself rather than the browser command catalog. The browser should still offer only non-authoritative convenience suggestions.

## Security, Privacy, and Reliability Requirements

Before expanding terminal reach beyond controlled internal use, implement the repository-required administrative dashboard authentication, explicit per-command authorization policy, operator identity in audit records, and configured command policy. The gateway must validate and bound every operator-authored field, including labels, command text, selected shell, working-directory action, cursor, history query, and idempotency key. It must enforce agent/session ownership, queue and concurrency limits, command expiry, and server-generated trace/audit IDs.

The client must validate the signed envelope, command type, expiry, replay protections, shell enum, capability generation when supplied, command length, and working-directory action before use. It must not construct an executable path from unvalidated operator input. Shell selection stays allowlisted and maps only to audited fixed argument vectors. The selected shell cannot be treated as proof that a command is safe; policy enforcement is gateway-owned and the local operating-system account remains authoritative for process permissions.

The capability catalog and terminal output can expose sensitive local facts. Apply per-field byte limits, per-agent catalog limits, redaction policy for logs and audit views, retention controls, no raw environment/PATH reporting, no implicit command execution for discovery, and no autocomplete recording outside the gateway audit path. Clipboard and rendering operations must treat output as untrusted text. Audit records must include operator identity when available, authenticated agent ID, server command/session IDs, selected shell, policy result, timestamps, and result classification, but not secrets, full environment data, or unbounded output.

Use TLS 1.3 and existing mTLS for all agent-gateway traffic. Capability data does not weaken certificate validation and does not substitute for mTLS-derived identity. All command discovery, command submission, cancellation, and results remain gateway-mediated.

## Data and API Contract Changes

Introduce versioned, typed contract additions rather than extending untyped maps.

| Resource                       | Required fields                                                                                                                                               | Data classification                                                                                            |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `terminal_capabilities`        | protocol version, client-observed time, gateway-received time, generation, TTL, default shell, bounded shell states, bounded command catalog, truncated flag  | Capability facts are client-authored; generation and received time are server-authored                         |
| `terminal_session`             | current shell, capability generation used at creation, last confirmed directory, lifecycle timestamps, optional display label                                 | IDs and timestamps are server-authored; confirmed shell/directory are client-authored results                  |
| `terminal_command`             | command ID, idempotency key, ordered state, request metadata, policy outcome, output streams, result classification, timestamps, truncation and timeout flags | IDs, lifecycle, and policy are server-authored; command is operator-authored; result facts are client-authored |
| `terminal_completion` response | capability generation, catalog freshness, bounded suggestions with kind and display token                                                                     | Client-authored data relayed by gateway; freshness metadata is server-authored                                 |

Extend the shared heartbeat schema and generated artifacts with bounded capability fields, then map them through client heartbeat construction, gateway ingestion, the activity directory, and the website client model. Do not expose the full heartbeat as a terminal API shortcut. The gateway should expose a dedicated read endpoint such as `GET /api/clients/{agentID}/terminal/capabilities`; it returns the capability generation and bounded catalog. Session creation accepts an optional normalized shell and expected capability generation. A mismatch, stale record, or unavailable shell returns a typed conflict response. Command creation accepts an idempotency key and returns a server-generated command lifecycle record.

Use structured error responses with stable codes. Do not return raw `exec` errors as the primary API contract. The browser may display a bounded diagnostic detail from the client result after HTML-safe rendering.

## Delivery Plan

### Phase 0: Contract and safety baseline

1. Document the current mode as a remote command workspace and update terminal documentation and UI wording where it implies interactivity.
2. Define shell, capability, command-state, and result-classification enums in the shared schema.
3. Replace fallback shell normalization with strict allowlist rejection in website, gateway, and client code.
4. Add bounded command lifecycle state, per-session ordering, submit idempotency, expiry, and structured failure mapping.
5. Add tests for unknown shells, stale capability generations, offline agents, queue saturation, output truncation, timeout, and result correlation.

### Phase 1: Capability discovery and shell correctness

1. Implement cached client discovery of fixed shell binaries using local executable lookup.
2. Add a bounded heartbeat capability report and gateway capability cache with freshness/TTL semantics.
3. Validate session-shell creation against current capabilities and revalidate on the client before each process launch.
4. Remove the website OS heuristic shell lists; populate the new-session selector from the gateway capability endpoint.
5. Render `shell_unavailable` distinctly, show its fresh available alternatives, and preserve the existing session selection without automatic mutation.

### Phase 2: Native-command completion

1. Implement bounded, non-recursive client discovery of executable basenames from the effective process search path.
2. Add reviewed static builtins for each supported shell and merge them with the executable catalog by normalized token.
3. Cache, relay, and expose the catalog through the dedicated gateway API with version, freshness, and truncation metadata.
4. Add the accessible first-token completion UI to the composer, with local filtering and explicit selection behavior.
5. Test catalog bounds, duplicate handling, unreadable PATH entries, missing shell binaries, stale catalog behavior, keyboard interaction, and no request-per-keystroke behavior.

### Phase 3: Reliable workspace operations

1. Replace implicit `cd` state mutation with a typed directory action and show the last client-confirmed directory.
2. Separate stdout and stderr, add cursor-paginated history, stable result codes, copy controls, retry/duplicate-into-composer, session rename, and session search.
3. Add gateway-owned persistence, retention, restart recovery, and append-only audit records before relying on history for operations.
4. Add gateway WebSocket or Server-Sent Events delivery for gateway-owned lifecycle updates; retain reconciliation from the read model after reconnect.
5. Add authenticated cancellation only after defining process-tree termination semantics for every supported platform.

### Phase 4: Policy and controlled expansion

1. Add authenticated operator identity, explicit command policy, approval/audit labels, rate limits, and configurable timeout/output/retention controls.
2. Add policy-aware UI states that explain rejection without exposing policy internals or secrets.
3. Evaluate argument and path completion only with a separate privacy review and explicit user opt-in where local names may be sensitive.

### Phase 5: PTY feasibility decision

1. Produce a separate PTY/ConPTY protocol design, threat model, capacity model, and operational runbook.
2. Prototype behind a disabled-by-default feature flag with strict session, frame, backpressure, timeout, cancellation, and retention limits.
3. Promote only after platform tests, gateway stream reconciliation, audit completeness, and security review are complete.

## Acceptance Criteria

- A Linux agent without `zsh` never presents `zsh` as selectable for a new session.
- A stale capability record prevents new shell selection and communicates that fresh agent information is required.
- A shell removed after discovery produces `shell_unavailable`, never a raw missing-executable message as the primary operator experience and never a silent fallback to another shell.
- Completion suggestions represent only bounded client-reported shell builtins and PATH executables for the selected agent and capability generation.
- The browser performs local suggestion filtering and does not issue a request for each typed character.
- An accepted command has an immutable gateway command ID, ordered lifecycle, expiry, bounded result, and an audit correlation record.
- Client-reported shells, executables, directories, output, and exit codes are never used as authorization or identity evidence.
- Session directory state changes only after an authenticated client result confirms the typed directory action.
- Restart, reconnect, cancellation, output truncation, nonzero exit, timeout, capability refresh, and stale state are represented explicitly and tested.
- No phase introduces direct browser-to-agent transport, permissive certificate behavior, raw PATH/environment disclosure, recursive filesystem discovery, or automatic shell/package installation.

## Validation Plan

Add table-driven client tests for fixed shell mapping, executable availability, missing binaries, cache expiry, command-catalog bounds, and structured outcomes. Add gateway tests for capability ingestion bounds, freshness checks, shell/session validation, ownership, idempotency, ordering, lifecycle transitions, pagination, retention, and audit correlation. Add website tests for available-shell rendering, disabled/stale states, suggestion filtering and keyboard behavior, error remediation, output-as-text rendering, and focus restoration.

Run generated-schema checks, formatting, unit tests, race tests for gateway/client state, linting, type checks, and production website build. Exercise at least Linux with `bash` only, Linux with `bash` and `zsh`, macOS with and without `zsh`, and Windows with `cmd`, Windows PowerShell, and PowerShell Core availability permutations. Validate that a capability report cannot change the agent selected by the gateway's mTLS identity binding.
