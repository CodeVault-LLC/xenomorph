# Roadmap to Milestone 1

Status date: 2026-07-14. The release decision and evidence baseline are maintained in `.docs/project-status.md`.

## Milestone Definition

Milestone 1 is an authorized internal preview of the Go gateway, Go agent, React administrative website, shared protocol, and secured broker. The supported deployment is bounded and documented. It is not public, Internet-facing, Common Criteria evaluated, or evidence of endpoint tamper resistance.

Work is ordered by dependency and trust. A later phase may be developed in parallel, but it cannot override an earlier acceptance gate or become an alternative top-level product.

## Phase 0: Governance and Build Baseline

Status: **complete**.

- `.docs/project-status.md`, this roadmap, `.docs/code-review.md`, `.docs/ci-and-release.md`, and `.docs/overview.md` own distinct status, planning, review, build, and architecture contracts.
- The root Makefile validates all Go modules and the website. Bun and `platform/website/bun.lock` remain the sole website package-manager contract.
- `rewrite` is the protected temporary integration branch. GitHub requires contributors to use a current branch, one fresh code-owner approval, resolved conversations, and both named CI jobs; force-push and deletion are disabled. Administrators retain an explicit small-change bypass governed by `.docs/code-review.md`.
- `.github/CODEOWNERS` assigns repository and component ownership. The pull-request template requires ownership, trust classification, compatibility, validation, and residual-risk evidence from `.docs/code-review.md`.
- All previously reported golangci-lint findings and the two subsequently exposed shared-module findings are resolved without disabling a linter. Key-service tests cover provider rejection, fail-closed lifecycle, DEK operation/failure, command signing, and protected key storage. Broker tests cover stream provisioning, unavailable state, protobuf publication, and synchronous JetStream acknowledgement failure.

Gate evidence: `make ci-go` and `make ci-web` pass locally on 2026-07-14. GitHub branch-protection verification on the same date reports the two required checks `Go quality and cross-platform build` and `Website quality and production build` on `rewrite`.

## Phase 1: Close the Gateway Trust Boundary

Status: **not started; release blocking**.

- Add authenticated operator identity and server-side authorization to every dashboard HTTP and WebSocket route.
- Bind terminal, screen, file, and administrative audit events to authenticated operator, agent, trace, request, and result identifiers.
- Add session expiry, revocation, cross-site request defenses, rate limits, and negative authorization tests.
- Configure NATS with independent mutual-TLS identity, subject authorization, secure JetStream administration, and bounded reconnect and publication failure behavior. Gateway publication already waits for the JetStream acknowledgement.
- Remove tracked development private keys from current source and history, rotate them, externalize enrollment and runtime secrets, and document incident handling for exposed development credentials.

Gate: threat-model review and integration tests prove unauthenticated, unauthorized, cross-agent, cross-origin, forged, expired, replayed, and broker-bypass actions fail closed.

## Phase 2: Stabilize Runtime and Recovery Contracts

Status: **partially implemented; release blocking**.

- Externalize and validate client gateway URL, TLS server name, credential location, heartbeat interval, polling/backoff, storage paths, and operational limits. Use a 10–30 second default heartbeat.
- Define gateway/client enrollment, renewal, revocation, shutdown, reconnection, offline, and upgrade state machines.
- Make durability choices explicit for command queues, replay state, operations, transfers, audits, screen/session state, and key metadata.
- Add backup, restore, retention, secure-deletion, corruption, restart, and partial-failure tests.
- Replace development compose inputs with pinned, authenticated, release-scoped deployment definitions or explicitly exclude containers from Milestone 1.

Gate: clean install, restart, backup/restore, degraded-dependency, and rollback exercises produce the documented state without identity confusion, silent command loss, or unbounded retry.

## Phase 3: Complete the File Workspace Contract

Status: **Phases 0, 2, and 3 substantially implemented; residual Phase 1 and Phases 4–5 incomplete**.

- Complete bounded search, cancellation, DOM virtualization, large-directory benchmarks, and the remaining Phase 1 acceptance evidence.
- Implement Phase 4 normalized metadata writes and archive create/list/extract only through explicit platform capability gates and archive traversal/bomb controls.
- Implement Phase 5 metrics, alerts, retention enforcement, controlled audit export, chaos/load/recovery tests, accessibility tests, and internal-user documentation.
- Run native integration suites for Linux, macOS, and Windows, including link/reparse races, cross-volume behavior, permissions, stale snapshots, transfers, permanent deletion, metadata fidelity, and archive limits.

Gate: every gate in `file-explorer-transfer-plan.md` passes. Cross-compilation is not native integration evidence.

## Phase 4: Complete Cryptographic Promotion Controls

Status: **software-provider foundation implemented; production controls incomplete**.

- Decide which cryptographic services are required for Milestone 1 and disable all others at the public contract until their complete controls are approved.
- Select an approved provider/operating environment and implement the applicable HSM/KMS custody, authenticated sessions, authorization, quota, alarm, recovery, destruction, and audit controls.
- Implement command-key version persistence, prepublication, rotation, client refresh, verification overlap, revocation propagation, rollback protection, and multi-instance behavior.
- Implement distributed nonce/key management and protected message integration before enabling any AES-GCM event encryption. Do not enable a partial data-protection path.
- Add the key-service unit, integration, failure-injection, sandbox, certification-evidence, and rotation tests required by `gateway-cryptographic-key-generation-plan.md`.

Gate: the exact build, provider, certificate, operating environment, policy, key lifecycle, failure behavior, and evidence package receive independent cryptographic and operations approval. No algorithm or deployment carries a broader claim than its evidence.

## Phase 5: Native Components and Packaging Decision

Status: **not specified**.

- Inventory any capability believed to require Rust or C and write an architecture decision for each accepted component.
- Define process isolation, privilege, IPC/FFI, resource bounds, platform ownership, crash behavior, logging, signing, updates, and rollback before code is added.
- Prefer Rust for new memory-sensitive helpers. Use C only for a documented platform or ABI requirement.
- Add pinned toolchains, format/lint/test/security gates, native platform tests, dependency inventory, SBOM, and signed packaging.

Gate: each native binary has an approved owner and threat model and cannot bypass or duplicate gateway authority. If no requirement survives review, Milestone 1 ships without additional native binaries.

## Phase 6: Product Documentation and Full Review

Status: **not started; release blocking**.

- Update overview, architecture, trust/data-flow diagrams, public contracts, configuration reference, deployment topology, operator model, threat model, and platform capability matrix from verified runtime behavior.
- Add enrollment, key/certificate lifecycle, monitoring, incident response, backup/restore, retention/deletion, upgrade/rollback, and disaster-recovery runbooks.
- Add bounded internal-user documentation for terminal, screen, logs, and file operations, including irreversible actions and platform limitations.
- Perform architecture, gateway/security, client/platform, website/accessibility, protocol/compatibility, cryptographic, operations, dependency, and documentation reviews under `.docs/code-review.md`.
- Record every Blocker and High finding with owner, resolution evidence, and independent closure.

Gate: documentation is implementation-aware, release evidence is complete, and no unresolved Blocker or High finding remains.

## Phase 7: Release Candidate and Promotion

Status: **blocked by Phases 1–6**.

- Freeze the supported component versions, operating systems, architectures, deployment configuration, dependencies, and explicit exclusions.
- Run clean-room CI plus native integration, end-to-end, accessibility, security, performance, chaos, recovery, install, upgrade, and rollback suites.
- Generate checksums, SBOMs, provenance, signatures, release notes, known limitations, and the milestone acceptance record.
- Stage and verify the exact artifacts, then require product, gateway-security, and operations approval.

Gate: `.docs/project-status.md` is reviewed and changed to ready; the protected signed tag points to the reviewed commit; every published artifact verifies against its checksum, signature, and provenance.

## Deferred Beyond Milestone 1

- Formal Common Criteria evaluation or any EAL claim.
- Client tamper-resistance or remote-attestation claims without a defined attacker model and supported hardware/OS baseline.
- Public or Internet-facing operation.
- Multi-tenant cryptographic isolation unless Milestone 1 explicitly adopts multi-tenancy.
- Interactive PTY behavior, remote-share connectors, managed file trash/restore, or other features not accepted into the bounded milestone contract.
