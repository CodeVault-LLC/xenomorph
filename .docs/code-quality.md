# Code Quality and Security Standard

## Purpose

This document defines the mandatory code quality, security, and performance rules for the Xenomorph repository. It applies to all Go code in `platform/client`, `platform/services/gateway`, `platform/shared`, and any future Go modules.

This standard exists because the gateway is the sole trust boundary and the client is an untrusted emitter. Code that is readable, minimally complex, and fast is not sufficient; it must also fail securely when the client, the operator dashboard, or an upstream dependency behaves maliciously or pathologically.

## Scope

- **Owned by this document:** style rules, security rules, performance rules, CI gates, and the classification of data as server-authored, client-authored, or operator-authored.
- **Not owned by this document:** build commands (see the root `Makefile`), operational runbooks, or certificate generation procedures.

## Mandatory Style Rules

The repository follows the Google Go Style Guide and Go Code Review Comments. The rules below are non-negotiable.

### Formatting and imports

1. All Go source files must be formatted with `gofmt`.
2. Import groups must be ordered: standard library, blank-line, third-party, blank-line, project-local.
3. Unused imports are prohibited.
4. Dot imports, aliased imports without justification, and import cycles are prohibited.

### Naming

1. Package names must be short, lowercase, and single-word. They must not contain underscores or mixedCaps.
2. Exported identifiers must use mixedCaps and must have a doc comment.
3. Unexported identifiers must use mixedCaps.
4. Acronyms and initialisms (`HTTP`, `URL`, `ID`, `TLS`, `NATS`, `JSON`) must keep consistent casing: `ServeHTTP`, `clientID`, `tlsConfig`, not `ServeHttp` or `clientId`.
5. Test helpers and table-driven tests must use descriptive names; avoid `Test1`, `TestFoo`, or `testData` when the intent can be explicit.

### Comments and documentation

1. Every exported function, method, type, constant, and variable must have a doc comment that explains what it does, not how it does it.
2. Comments must be complete sentences and end with a period.
3. Internal packages must document their ownership boundary: what the package owns, what it does not own, and the trust source for any identity assertion.
4. Comments must not state the obvious; they must state intent, invariants, and failure modes.

### Packages and visibility

1. Prefer `internal/` packages for code that is not part of a public API.
2. Do not export symbols solely for tests. If a symbol must be testable, reconsider the design or place tests in the same package.
3. Package-level variables must be immutable after init. Package-level mutable state is prohibited unless it is a thread-safe cache with explicit bounds and a documented reason.
4. Package `init` functions are prohibited unless there is no other way to register a handler or build tag hook.

### Errors

1. Errors must be wrapped with `fmt.Errorf("...: %w", err)` when crossing package boundaries.
2. Sentinel errors must be declared as package-level `var ErrSomething = errors.New("...")` and documented.
3. Errors must be checked. `_ = something()` is prohibited unless justified in an adjacent comment.
4. Error strings must be lowercase and must not end with punctuation.
5. Do not log and return the same error without adding context.

### Context

1. `context.Context` must be the first parameter of every function that performs I/O, cancellation, or request-scoped work.
2. Do not store `context.Context` in a struct field.
3. Context keys must use an unexported typed key, never a raw string:

```go
type ctxKey string
const traceIDKey ctxKey = "trace_id"
```

### Functions and control flow

1. Functions should do one thing. As a guideline, functions must not exceed 60 lines; functions that exceed 80 lines require explicit justification.
2. Cyclomatic complexity must not exceed 10, measured by `golangci-lint cyclop`. Functions that exceed this must be refactored.
3. Naked returns and named result parameters are prohibited unless they materially improve readability.
4. `panic` is prohibited in production code. Return errors.
5. `goto`, `defer` inside loops, and shadowing of builtin names are prohibited.

### Interfaces

1. Define interfaces at the point of use, not the point of implementation, unless the interface is a public contract.
2. Keep interfaces small. A good Go interface has one or two methods; three is acceptable; more requires justification.
3. Do not use interfaces to abstract a single concrete type.

### Concurrency

1. Every goroutine must have a documented lifetime and a clean shutdown path.
2. Use `sync.WaitGroup`, `errgroup.Group`, or explicit channel signaling; do not leak goroutines.
3. Shared state must be protected by mutexes, atomics, or channels. Race conditions are not acceptable; `go test -race` must pass.
4. Channels must have an owner: the goroutine that sends, the goroutine that receives, or a documented manager.

### Testing

1. Tests must be table-driven where there are more than two cases.
2. Tests must not depend on global mutable state, the network, the filesystem, or the system clock unless explicitly isolated.
3. Tests must use `t.Parallel()` where safe.
4. Test helpers must call `t.Helper()`.
5. Coverage is required for business logic. `cmd/` packages and generated protobuf code are exempt, but their dependencies must be tested.

### JSON and serialization

1. Use typed structs for request and response bodies. `map[string]any` is prohibited for externally visible JSON shapes.
2. Use `json.RawMessage` only when the payload type is selected at runtime and validated before use.
3. Do not use `interface{}` in public APIs unless required by a protocol. Prefer concrete types or sum types.

### HTTP

1. Use `http.MethodPost`, `http.MethodGet`, and related constants; do not use string literals.
2. HTTP clients must have explicit timeouts.
3. Response bodies must be closed, even when the response status is non-2xx.
4. Request bodies must be bounded.

## Mandatory Security Rules

Security is not a feature. The gateway is the only component that may assert identity or trust. The client is untrusted at all times.

### Trust boundary

1. The gateway owns authentication, identity derivation, authorization, event provenance, and command dispatch.
2. The client owns local execution and telemetry collection. It does not own identity, authentication, or command authenticity.
3. The browser dashboard owns operator interaction. It does not own agent identity, command delivery, or command result authenticity.
4. No component may bypass the gateway for event ingestion or command issuance.

### Data classification

Every data field must be classified in code and comments:

- **Server-authored:** generated by the gateway. Examples: `agent_id`, `event_id`, `session_id`, command IDs, timestamps in `EventEnvelope`, `Signature`.
- **Client-authored:** generated by the agent. Examples: telemetry, hostname, screenshots, terminal output, log messages.
- **Operator-authored:** generated by the browser dashboard or Discord slash commands. Examples: terminal command text, screenshot requests, channel cleanup commands.

Server-authored fields may be used for trust decisions. Client-authored and operator-authored fields must never be used as identity evidence, authorization evidence, or host integrity evidence.

### Input validation

1. Every client-authored and operator-authored input must be validated at the trust boundary before use.
2. String fields must be trimmed and clamped to a maximum byte length appropriate for the downstream consumer.
3. Numeric fields must be range-checked.
4. Enumerated fields must be compared against an allowlist.
5. File paths and command strings must be validated to prevent traversal, injection, and unintended execution.
6. JSON payloads must be decoded into bounded readers; do not decode unbounded request bodies.

### Authentication

1. Agent-to-gateway transport must use mutual TLS with `ClientAuth: tls.RequireAndVerifyClientCert` and `MinVersion: tls.VersionTLS13`.
2. Server certificate name validation must be configured with an explicit `ServerName` on the client side; it must not default to `localhost` in production.
3. The dashboard is an administrative interface. It must authenticate operators before exposing agent data or accepting commands. Plain HTTP without authentication is not acceptable.
4. Service-to-service communication (gateway to NATS) must support TLS with client certificates.

### Authorization

1. Agents may only access their own command queue and may only submit results for their own identity.
2. Operators may only act on agents that exist in the gateway directory.
3. Terminal sessions and screenshots must be scoped to a single agent identity.

### Command authenticity

1. Command envelopes dispatched to agents must carry a cryptographic signature or be delivered exclusively over the mutually authenticated channel.
2. A literal string signature such as `"gateway"` is not acceptable.
3. Agents must verify the signature before executing any command.
4. Commands must have a monotonic expiry; expired commands must be rejected.

### Subprocess execution

1. The client must not execute shell commands constructed from operator input without validation.
2. Shell selection must be normalized against an allowlist.
3. Working-directory changes must be validated against the filesystem; traversal outside the intended scope must be rejected.
4. Where possible, avoid shell intermediaries and invoke binaries directly with validated arguments.

### WebSockets

1. WebSocket upgrader `CheckOrigin` must not return `true` for all origins.
2. The allowed origin must be configurable and default to the dashboard origin.

### Secrets and configuration

1. Secrets must be loaded from environment variables, secret files, or a secret manager. They must not be checked into source control.
2. Certificate paths, gateway addresses, and timeouts must be configurable.
3. Default certificate paths must not rely on relative paths such as `../../infrastructure/certs` in production.

### Audit logging

1. Operator actions that affect agent state (terminal commands, screenshot requests, session deletion, Discord cleanup) must be logged with operator identity, agent identity, trace ID, and timestamp.
2. Client actions (heartbeats, command results, log entries) must be logged at the gateway boundary with the authenticated agent identity.
3. Logs must not contain secrets, tokens, or private keys.

### Cryptography

1. Deterministic identifiers derived from certificates must use a documented algorithm and must not collide across certificates.
2. All TLS must use TLS 1.3.
3. Avoid custom cryptographic primitives.

## OWASP ASVS Level 2 Mapping

The following ASVS Level 2 requirements are mandatory for this codebase.

### V1 Architecture, design and threat modeling

- V1.2.1: All components and interfaces are documented, including the trust boundary.
- V1.2.2: Security controls are enforced server-side and are not bypassable by the client.
- V1.4.1: The architecture enforces least privilege for all components.

### V2 Authentication

- V2.2.1: mTLS is used for agent authentication.
- V2.2.4: Weak or default credentials are not used.

### V3 Session management

- V3.1.1: Session identifiers are generated by the gateway and are unpredictable.
- V3.3.2: Sessions are invalidated after a period of inactivity.

### V4 Access control

- V4.1.1: Access control is enforced at the gateway.
- V4.1.2: Administrative functions require authentication.

### V5 Validation, sanitization and encoding

- V5.1.3: All input is validated using positive validation (allowlists).
- V5.2.1: Output encoding is used when rendering client data.
- V5.2.4: Command injection defenses are in place for terminal and screenshot execution.

### V6 Stored cryptography

- V6.2.2: Keys are protected from unauthorized disclosure.
- V6.3.1: TLS 1.3 is enforced for all network communication.

### V7 Error handling and logging

- V7.1.1: Error messages do not disclose sensitive information.
- V7.1.2: All security-relevant events are logged.

### V8 Data protection

- V8.1.1: Sensitive data is classified and handled according to classification.
- V8.2.1: Sensitive data is encrypted in transit.

### V9 Communications

- V9.1.2: TLS settings use strong cipher suites and versions.
- V9.1.3: Certificate validation is strict.

### V10 Malicious code

- V10.1.1: Code review is required for security-sensitive changes.
- V10.2.1: Application source code and dependencies are free of malicious code.

### V11 Business logic

- V11.1.1: Business logic flows cannot be bypassed.
- V11.1.2: High-value transactions require authorization.

### V12 Files and resources

- V12.1.1: File upload paths and names are validated.
- V12.3.1: File inclusion vulnerabilities are prevented.

### V13 API and web service

- V13.1.1: APIs enforce authentication and authorization.
- V13.2.1: API inputs are validated.

### V14 Configuration

- V14.1.1: Build and deployment processes are scripted and repeatable.
- V14.2.1: Default configurations are secure.
- V14.3.1: Sensitive configuration values are externalized.

## NIST SSDF Protect Software Practices

The repository must satisfy the following NIST Secure Software Development Framework practices under the `Protect Software (PS)` category.

### PS.1 Protect software from unauthorized access

1. Source code and build pipelines must be accessible only to authorized contributors.
2. Secrets, certificates, and runtime credentials must be stored in secret managers or environment-specific files, never in source control.
3. Administrative interfaces (dashboard, Discord bot configuration) must require authentication.

### PS.2 Protect software from tampering

1. Build outputs must be produced by the repository `Makefile` or CI pipeline, not ad-hoc local commands.
2. Go module checksums (`go.sum`) must be committed and verified.
3. Container images, if introduced, must be built from pinned base images and signed.
4. Command envelopes dispatched to agents must be integrity-protected so a client can detect tampering.

### PS.3 Verify released software integrity

1. Releases must be tagged and reproducible.
2. Cryptographic checksums of binaries must be published with releases.
3. The CI pipeline must run `go test -race`, `go vet`, `golangci-lint`, and `go mod tidy` before any release.

## Mandatory Performance Rules

The system must remain fast and predictable as the number of agents and operators grows.

### Telemetry cadence and caching

1. Heartbeat interval must be configurable. The default must be between 10 and 30 seconds, not sub-second.
2. Static telemetry (CPU model, core count, GPU list, OS version, installed applications, browser list) must be cached for at least the process lifetime or a documented TTL.
3. Semi-static telemetry (network interface name, link speed) must be cached for seconds to minutes, not recomputed on every heartbeat.
4. Volatile telemetry (CPU load, RAM usage) may be sampled per heartbeat.

### Bounded resources

1. All in-memory caches must have per-agent or global bounds and documented eviction behavior.
2. All goroutines spawned per request or event must be bounded by a semaphore, worker pool, or explicit limit.
3. All queues must have a maximum depth. Enqueue operations that exceed the limit must drop the oldest or newest item and log the event.
4. HTTP request and response bodies must be bounded.

### Efficient serialization and I/O

1. Avoid decoding and re-encoding media. If the client captures screenshots in PNG and the dashboard needs JPEG, prefer capturing JPEG directly when the platform tool supports it.
2. Use protocol buffers for broker messages; do not send JSON to NATS.
3. NATS publishes must be synchronous or must await the async publish acknowledgment before returning success to the caller.
4. Database or store operations, if introduced, must use connection pooling and bounded contexts.

### Avoiding allocation pressure

1. Reuse buffers where hot paths allocate heavily.
2. Pre-size slices and maps when the expected size is known.
3. Avoid `fmt.Sprintf` in hot paths; use structured logging instead.

### Observability for performance

1. Add metrics for heartbeat rate, command queue depth, NATS publish latency, Discord API latency, screenshot processing duration, and dashboard viewer count.
2. Use histograms for latency and counters for throughput.
3. Health endpoints must distinguish between liveness and readiness.

## Enforcement

### Local checks

Before opening a pull request, contributors must run:

```bash
make fmt
make tidy
make test
make build
make lint
```

### CI gates

A change may not merge unless all of the following pass:

1. `go test -race ./...`
2. `go vet ./...`
3. `golangci-lint run`
4. `go mod tidy` produces no diff
5. `make build`
6. All new exported identifiers have doc comments
7. All new security-sensitive code has a corresponding test or threat-model note

### Lint configuration

The repository uses `golangci-lint` with the following mandatory linters:

- `bodyclose`
- `cyclop`
- `errcheck`
- `goconst`
- `gosec`
- `govet`
- `ineffassign`
- `mnd`
- `revive`
- `staticcheck`
- `unused`

Generated protobuf code under `platform/shared/proto/gen` is excluded from linting.

## Sources

The rules in this document are derived from the following sources:

1. Google. *Google Go Style Guide*. https://google.github.io/styleguide/go/
2. The Go Authors. *Go Code Review Comments*. https://go.dev/wiki/CodeReviewComments
3. The Go Authors. *Effective Go*. https://go.dev/doc/effective_go
4. Rob Pike. *Go Proverbs*. https://go-proverbs.github.io/
5. Open Web Application Security Project. *OWASP Application Security Verification Standard 4.0*. https://owasp.org/www-project-application-security-verification-standard/
6. National Institute of Standards and Technology. *Secure Software Development Framework (SSDF) Version 1.1*. https://csrc.nist.gov/projects/ssdf
7. Uber Technologies. *Uber Go Style Guide*. https://github.com/uber-go/guide/blob/master/style.md
8. The Go Authors. *The Go Memory Model*. https://go.dev/ref/mem
9. OpenSSF. *OpenSSF Best Practices Badge Program*. https://www.bestpractices.dev/
