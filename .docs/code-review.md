# Code Review Standard

## Purpose and Authority

This document defines the required review workflow for human and automated contributors. It owns review evidence, change hygiene, naming, and language-specific review checks. `.docs/code-quality.md` remains authoritative for Go, security, performance, OWASP ASVS Level 2, and NIST SSDF requirements.

A reviewer approves only the behavior represented by the diff and its validation evidence. Approval does not convert client-authored or operator-authored data into trusted data and does not waive a release gate.

## Change Preparation

Before editing, the author or automation must:

1. Identify the owning component and its non-ownership boundary.
2. Read `AGENTS.md`, `.docs/code-quality.md`, the relevant runtime specification, and the current roadmap item.
3. Inspect the existing public and persistence contracts before changing them.
4. Classify changed inputs and fields as server-authored, client-authored, or operator-authored.
5. State the smallest validation command that can fail the proposed change quickly.

After the first substantive edit, run that validation. Before review, run every required gate for the touched component.

## Branch, Commit, and File Naming

Branches use `<type>/<short-kebab-case-scope>`. Allowed types are `feature`, `fix`, `security`, `docs`, `refactor`, `test`, `build`, `ci`, and `chore`. Examples are `security/operator-authentication` and `ci/website-quality-gate`. Protected integration and release branches are not used as personal work branches.

Commit subjects use `<type>(<scope>): <imperative summary>`, following Conventional Commits. Use a body when the change affects a trust boundary, public contract, migration, release gate, or non-obvious tradeoff. The body states why the behavior changed and the security or compatibility consequence. Commits must be reviewable and coherent; do not combine formatting or unrelated cleanup with a behavioral change.

Naming contracts are:

| Artifact | Convention | Example |
| --- | --- | --- |
| Go files | lowercase `snake_case.go` when multiple words are needed | `transfer_store.go` |
| Go packages | short, lowercase, one word | `keyservice` |
| Documentation | lowercase `kebab-case.md` | `release-readiness.md` |
| TypeScript and React files | lowercase `kebab-case.ts` or `.tsx` | `transfer-drawer.tsx` |
| React components and exported TypeScript types | `PascalCase` | `TransferDrawer` |
| TypeScript functions, hooks, and variables | `camelCase`; hooks start with `use` | `useAgentLogs` |
| Environment variables | uppercase `SNAKE_CASE` | `NATS_URL` |

Generated files are changed only by their owning generator. A generated diff must be paired with the source contract change and a reproducible generation command.

## Change Workflow Selection

Every AI-assisted change is classified before editing. Classification is based on review structure and risk, not line count.

### Large-change workflow

A change is large when any of the following applies:

- it crosses component ownership boundaries or changes a public, protocol, persistence, deployment, or trust-boundary contract;
- it contains two or more behavior slices that can be implemented and validated independently;
- it requires a migration, staged rollout, compatibility period, or coordinated documentation update; or
- the user requests a multi-commit or feature-development workflow.

The author branches from the current integration branch. After the first meaningful and internally consistent commit, the author pushes and opens a draft pull request; empty bootstrap commits are prohibited. Subsequent commits represent coherent human-reviewable behavior slices and leave the branch buildable. Tests and documentation travel with the behavior they prove rather than accumulating in a final cleanup commit.

Published commits are not amended, squashed, force-pushed, or rebased merely to make the branch appear finished. The pull request is the continuing review record. Feedback received before merge is addressed on the same branch with new focused commits, and the pull-request description and validation evidence are updated. The completed pull request is handed to the user for manual squash-merge unless the user explicitly delegates merging.

### Small-change workflow

A change is small only when it is one localized behavior or documentation correction, fits one coherent commit, and does not change a trust boundary, public contract, migration, or release gate. A short branch with one commit and a pull request remains the default when CI or review visibility is useful.

Direct integration-branch commits are an explicit administrator path, not the default. Automation may use it only when the user requests it, the diff contains no unrelated work, focused validation passes before commit and push, and the branch rule permits the acting administrator to bypass pull-request requirements. If a task grows to a second independently reviewable behavior slice, automation creates a work branch before committing and follows the large-change workflow.

Both workflows use the same quality and security standard. “Small” reduces workflow overhead; it does not reduce validation, trust-boundary review, or documentation accuracy.

## Review Priority and Finding Severity

Review in this order: trust-boundary correctness, authentication and authorization, command authenticity, secret handling, input and resource bounds, data loss and recovery, concurrency, public compatibility, test evidence, then maintainability and style.

- **Blocker:** permits gateway bypass, unauthenticated administrative action, identity forgery, command forgery, secret disclosure, arbitrary execution beyond the documented contract, unrecoverable corruption, or a false production/compliance claim.
- **High:** violates a mandatory security control, permits cross-agent access, creates an unbounded external-input path, breaks durable state, or lacks tests for a security-sensitive behavior.
- **Medium:** produces incorrect behavior, weakens error/recovery semantics, causes a compatibility break without migration, or materially degrades accessibility or performance.
- **Low:** localized maintainability, naming, documentation, or test clarity issue with no current correctness or security impact.

Every finding names the affected file and behavior, explains the consequence, and states the condition that demonstrates resolution. Reviewers do not hide security findings inside optional style feedback.

## Go Review

Apply `.docs/code-quality.md` in full. Confirm that I/O accepts context, bodies and queues are bounded, errors cross package boundaries with context, goroutines have shutdown paths, externally visible serialization is typed, and security-sensitive behavior has negative tests. Public contract changes require compatibility analysis across client, gateway, shared schema, website, persistence, and deployed versions.

Required evidence for Go changes is the relevant package test immediately after editing and `make ci-go` before merge. Platform-specific filesystem or process behavior also requires execution on every affected operating system; cross-compilation alone is not behavioral evidence.

## TypeScript and React Review

The website is an administrative presentation and intent-collection component. It does not authenticate agents, authorize operations, validate command authenticity, or establish the truth of client filesystem, screen, terminal, or telemetry data.

Reviewers enforce the following:

- Bun and `bun.lock` are the sole dependency contract. Dependency changes explain need, provenance, maintenance impact, browser/runtime exposure, and lockfile diff.
- External responses use explicit TypeScript types and runtime parsing where a malformed value can affect navigation, rendering cost, state-changing actions, or security. `any`, unchecked assertions, and open-ended external maps are prohibited.
- Untrusted values render through React text escaping. Raw HTML, dynamic script/style injection, unsafe URL schemes, and secret-bearing browser storage are prohibited.
- TanStack Query owns gateway server state and invalidation. TanStack Router owns navigable URL state. Zustand or component state owns only client-local state. A second cache of server truth requires an explicit consistency contract.
- Effects synchronize external systems; they do not duplicate derived state or hide action sequencing. Async effects and handlers cancel or ignore stale work and surface bounded failures.
- Mutations prevent accidental duplicate submission, represent pending/success/partial/failure states, and reconcile with gateway-authored operation state rather than assuming client success.
- Lists, previews, terminal output, logs, and media remain bounded. Rendering does not recursively enumerate remote trees or retain unbounded histories.
- Interactive elements have semantic roles, labels, keyboard access, visible focus, and focus restoration. Destructive flows use explicit irreversible language and the gateway/agent precondition flow.
- Client-side visibility and disabled states are usability controls only. Every authorization and validation decision is enforced by the gateway.

Required evidence is `make ci-web`. Changed user workflows additionally require focused component or end-to-end tests once the repository introduces those harnesses; the absence of a current browser test harness is a milestone gap, not permission to omit future tests.

## Rust, C, and Native Binary Review

No Rust or C production component currently has an approved ownership or runtime contract. Before introducing one, add an architecture decision that defines its purpose, supported platforms, privilege level, process and memory boundary, update/signing mechanism, failure behavior, and exact gateway/client interface. A native binary must not duplicate gateway authority or introduce a direct ingestion/control path.

Rust uses stable toolchains pinned in the repository, denies warnings in CI, formats with `rustfmt`, lints with Clippy, and contains unsafe code only in narrowly documented modules with invariants and tests. C uses a pinned compiler baseline, warning-as-error builds, sanitizers in test jobs, explicit ownership/lifetime documentation, and no unbounded string or memory operations. Both require dependency/license review, platform tests, artifact checksums, signing, and software-bill-of-material evidence before release.

## AI and Automation Workflow

Automation must:

1. Read repository instructions and the owning specification before acting.
2. Inspect the working tree and preserve unrelated user changes.
3. State assumptions that alter scope; do not invent an absent feature or runtime guarantee.
4. Make the smallest coherent change and validate the touched slice immediately.
5. Review its own diff for trust classification, secrets, destructive behavior, generated files, compatibility, and documentation accuracy.
6. Run the required component gate and report commands, failures, untested platforms, and residual risks truthfully.
7. Use the branch, file, and commit conventions in this document. Automation does not commit, push, open a pull request, rotate credentials, or change external state unless the user authorizes that action.

Automation must stop and request direction when completion requires a new product authority, an unchosen native-component boundary, a destructive data migration, credential rotation, or a security tradeoff not approved by the repository.

## Pull Request Gate

A pull request is ready for approval only when it includes:

- the owned component and linked roadmap item;
- a concise behavioral description and explicit non-goals;
- trust-source and data-classification changes;
- public API, persistence, deployment, and rollback impact;
- tests and exact validation commands;
- threat-model or security-standard updates for sensitive changes;
- screenshots or interaction evidence for visible website changes without using them as correctness evidence; and
- remaining risks, deferred work, and platform coverage.

Release-gate changes require two-person review, including a reviewer responsible for the gateway trust boundary. A change cannot merge while a Blocker or High finding remains unresolved.

The repository pull-request template collects this evidence. `CODEOWNERS` routes all changes to the current repository owner and records component paths explicitly; it does not replace specialist review or the two-person release-gate requirement.
