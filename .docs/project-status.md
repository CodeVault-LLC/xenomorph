# Project Status and Milestone 1 Readiness

Status date: 2026-07-15.

## Release Decision

**NOT READY FOR RELEASE.**

The current repository is a substantial Go implementation with a working gateway, agent, shared protocol, and React administrative website. It is suitable for continued development and bounded local evaluation. It is not a production-ready or public release because administrative authority, broker transport, credential lifecycle, cryptographic promotion evidence, operating-system validation, recovery, and release provenance are incomplete.

Milestone 1 is defined as an **authorized internal preview** of one gateway deployment, explicitly enrolled agents, the administrative website, and a secured NATS deployment. It is not an Internet-facing service, a Common Criteria evaluated product, a tamper-resistant endpoint claim, or a production cryptographic-module approval.

## Implemented Baseline

| Area | Present evidence | Boundary |
| --- | --- | --- |
| Agent identity | Gateway requires TLS 1.3 mutual TLS and derives agent ID from the verified certificate fingerprint. | A valid certificate authenticates the credential, not the truth or integrity of client telemetry. |
| Event ingress | Gateway authors event identity and provenance before protobuf publication to NATS JetStream. | Client payload remains client-authored. |
| Command integrity | Dedicated RSA-PSS command key, signed expiring envelopes, audience binding, and persisted replay protection exist. | Automated rotation and protected provider-backed key custody do not exist. |
| File workspace | Automatic roots, paged listing, metadata read, bounded preview, durable single-file transfer, safe mutations, dry-run deletion, and permanent deletion exist. | Phase 1 search/virtualization evidence and all Phase 4/5 acceptance gates are incomplete. Filesystem reports remain client-authored. |
| Operator website | Agent, logs, terminal, screen, and file workflows build successfully in React and TypeScript. | No authenticated human operator or authorization layer exists. A configured operator label is audit-only. |
| Cryptographic service | Software-provider validation, fail-closed readiness, opaque key handles, command-key separation, cryptographic consistency probes, lifecycle tests, nonce-failure tests, and protected command-key storage tests exist. | The key-generation plan explicitly records partial and missing production controls. |
| Broker publication | Gateway events are protobuf messages published synchronously to JetStream; publication succeeds only after a broker acknowledgement. Stream provisioning and acknowledgement failures have direct tests. | The gateway-to-NATS connection still lacks independent mutual-TLS identity, subject authorization, and bounded reconnect policy. |
| Agent transport candidate | Raw QUIC v1, strict TLS 1.3 mTLS, XBP generation/codecs, bounded streams, server-pushed commands, durable replay/journals, transfer/media adapters, admission/fencing, metrics, and real-listener trust tests are implemented. The existing dashboard and broker protobuf contract are unchanged. | The listener is disabled by default. XBP benchmark approval, UDP deployment/capacity/soak/native-OS evidence, certificate lifecycle, canary, and independent reviews are incomplete. HTTP/WebSocket compatibility routes remain. |
| CI and review | Go formatting, tests, race, vet, static/security analysis, vulnerability scan, lint, cross-build, and website format/lint/type/build gates pass locally. Contributor changes to `master` require both CI jobs, a fresh code-owner review, a current branch, and resolved conversations. Large AI changes always use that PR workflow; administrators retain a documented small-change bypass. | Platform integration, browser tests, signing, provenance, SBOM, and release publication do not exist. |

## Milestone 1 Blockers

The following are release-blocking. Priority is determined by trust and operational dependency, not by feature visibility.

1. **Broker protection:** use independently issued NATS mutual-TLS credentials, authenticate and authorize the gateway subject namespace, bound reconnect/publish behavior, and document broker recovery. Synchronous durable publish acknowledgement is implemented and tested.
2. **Credential remediation:** remove development CA and private keys from tracked source, purge them from history where they may have been exposed, revoke/rotate them, provide an external enrollment and secret-delivery process, and ensure release packages exclude credentials and local state.
3. **Transport production evidence:** select the authoritative agent transport and codec from controlled evidence; approve the exact QUIC/Go/cryptographic boundary; prove UDP/firewall/NAT/kernel behavior, capacity, adversarial and soak behavior, mixed-version rollback, certificate lifecycle, and native client platforms. The current listener remains disabled by default. Runtime endpoint, credential, cadence, state, timeout, and fallback configuration is externalized and validated; production deployment values remain an operations-owned input.
4. **Cryptographic production gate:** resolve every applicable partial/missing control in `gateway-cryptographic-key-generation-plan.md`, extend failure injection and provider-integration evidence, select the actual HSM/KMS or explicitly approved provider and operating environment, implement rotation/recovery/audit controls, and produce independent approval evidence. Disabled future encryption algorithms need not be exposed merely to satisfy the milestone; no unimplemented algorithm may carry a production claim.
5. **File workspace completion:** complete the residual Phase 1 search/virtualization and benchmark gate, Phase 4 metadata/archive safety work, and Phase 5 metrics, retention, audit export, chaos/load/recovery/accessibility work. Test mutations and transfers natively on every supported OS.
6. **Durability and recovery:** command/operation journals and the authenticated client replay ledger now define interrupted-command behavior. Complete persistence choices for dashboard state, audits, key metadata, backup, restoration, retention, secure deletion, and whole-system partial failure; prove the complete deployment recovery contract.
7. **Release engineering:** implement the pipeline in `.docs/ci-and-release.md`, pin dependencies and actions, add platform/browser/integration suites, sign artifacts and provenance, publish checksums/SBOMs, and prove clean install/upgrade/rollback.
8. **Documentation and review evidence:** complete enrollment, certificate lifecycle, deployment-specific, user, and release documentation; perform the full review defined by `.docs/code-review.md`; close all Blocker and High findings.

## Decisions Required Before New Native Binaries

The repository contains no approved requirement or runtime contract for a C or Rust binary. Milestone planning must not assume that additional binaries improve security by their existence. Before implementation, an architecture decision must identify the exact missing capability, why the Go process cannot safely own it, supported platforms, privilege and sandbox boundary, IPC/FFI contract, memory-safety model, signing/update path, and failure behavior.

Rust is preferred for new memory-sensitive native code unless a platform API, verified library, or ABI constraint requires C. A native helper remains subordinate to the agent and cannot authenticate operators, assert agent identity, sign gateway commands, or publish around the gateway.

## Common Criteria Position

`.docs/explorations/common-criteria-evaluation.md` remains an exploration and must not become a launch claim. Formal evaluation is deferred until the target of evaluation, attacker model, supported configuration, operator boundary, scheme, and required assurance decision are stable. Milestone 1 should adopt the exploration's evidence discipline and gap list without claiming an EAL or Common Criteria conformance.

## Exit Criteria

Milestone 1 may be declared ready only when every blocker has an owner, evidence link, and approved result; `make ci` and all added platform/integration suites pass from a clean checkout; no production credential or runtime state is tracked; release artifacts are signed and verified; operational rollback and recovery are demonstrated; and the project status is changed through reviewed evidence rather than schedule pressure.
