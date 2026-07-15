# Terminal Sessions

## Ownership

The terminal feature is owned by three components:

- `platform/website` owns the browser workspace for selecting an existing terminal session, starting a new blank terminal state, submitting commands, displaying queued and completed command entries, and requesting session deletion from the gateway.
- `platform/services/gateway` owns the authorization boundary, terminal session read model, command queue publication, and authenticated result ingestion.
- `platform/client` owns local shell process execution after it receives a server-authored command from the gateway over the authenticated agent channel.

The browser does not own agent identity, command delivery, command result authenticity, or direct agent transport. The client does not own authenticated identity assertions. The shared event schema remains a broker-level schema; the terminal dashboard read model is gateway-local process memory in the current implementation.

## Trust Model

Agent identity is gateway-authored from the mTLS peer certificate and is derived inside the gateway middleware. Browser-supplied agent identifiers are routing selectors only and are accepted only when the gateway directory knows the authenticated agent.

Terminal session identifiers, command identifiers, submission timestamps, and queued status are gateway-authored. Command text and requested shell are user-authored operator input from the website. Working directory is gateway-selected by default and then updated by client-authored terminal command results such as `cd`. Command output, exit code, confirmed shell, confirmed working directory, and client hostname are client-authored results returned over the authenticated mTLS result path.

Client-authored terminal output is not trusted input. It is displayed as remote process output and must not be used as identity evidence, authorization evidence, or host integrity evidence.

## Runtime Flow

1. The website loads existing sessions through `GET /api/clients/{agentID}/terminal/sessions`.
2. When an operator submits a command with no selected session, the website first calls `POST /api/clients/{agentID}/terminal/sessions` to create a gateway-local session. The operator does not need to create a session before typing.
3. The website calls `POST /api/clients/{agentID}/terminal/sessions/{sessionID}/commands` with command text.
4. The gateway verifies that the agent is known, online, and that the terminal session belongs to the selected agent.
5. The gateway enqueues a `support.terminal.run` command with a gateway-authored command ID.
6. The gateway pushes the signed command over the authenticated QUIC command lane. The client validates and executes it through the selected local shell, then returns the result over the QUIC event lane.
7. The gateway stores the result in the terminal read model and exposes it through `GET /api/clients/{agentID}/terminal/sessions/{sessionID}/entries`.

## Current Behavior

The client supports multiple terminal sessions by keeping per-session shell and working directory state in memory. `cd` is handled as a built-in state update so the next command in that session uses the new directory. Other commands execute as bounded subprocess invocations through the requested shell.

The website displays sessions as compact top-level tabs. A blank terminal state is started from the plus button and becomes a gateway session only after the first command is submitted. When multiple shell labels are available for the reported operating system, the blank terminal state exposes a shell selector before the first command. Session metadata is hidden from the primary terminal view; the session options menu exposes administrative actions such as copying history, copying the session identifier, and deleting the session.

Supported shell labels are `bash`, `zsh`, `sh`, `powershell`, `pwsh`, and `cmd`. The gateway defaults to `powershell` for Windows, `zsh` for macOS, and `bash` for other operating systems. The client normalizes shell labels again before execution.

Command output is bounded before submission. Long-running commands are terminated by the client timeout. The gateway stores terminal sessions and entries in memory; data is lost when the gateway process restarts. Deleting a session removes its in-memory entries from the dashboard read model.

## Current Non-Ownership

The implementation does not provide a raw interactive PTY, terminal resize events, binary terminal protocols, job control, curses/full-screen applications, stream-by-stream stdout/stderr separation, or persistent terminal history. It does not bypass the gateway and does not expose arbitrary browser-to-agent sockets.

## Roadmap

### Priority 1

- Add a PTY-backed client execution mode for Unix and Windows ConPTY so interactive programs, resize events, and control sequences behave like a native terminal.
- Replace dashboard polling with a gateway WebSocket stream that carries terminal output frames, command lifecycle events, and session status updates.
- Add explicit per-command authorization policy and audit labels before enabling terminal commands outside controlled internal environments.

### Priority 2

- Persist terminal sessions and command audit records in a gateway-owned store with retention controls.
- Split stdout and stderr in the command result model.
- Add command cancellation from the website through a gateway-authored cancellation command.

### Priority 3

- Add shell capability discovery from the client heartbeat or endpoint attestation.
- Add configurable output limits, timeout limits, and blocked command patterns.
- Add UI affordances for command history search and session renaming.
