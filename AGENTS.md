# AGENTS.md

## Mission

This repository implements an internal remote screening platform composed of an agent, a gateway, and a shared event schema. Work in this tree must preserve the integrity of the trust boundary at the gateway and must maintain the repository as a controlled, authorized system.

## Mandatory Working Pattern

1. Identify the owning component before editing.
2. Verify the local runtime contract before changing a public interface.
3. Prefer the smallest coherent edit that preserves behavior.
4. Validate the touched slice immediately after the first substantive change.
5. Stop only after the repository state is internally consistent and the requested outcome is complete.

## AI Change Workflow

Before editing, classify the request as a **large change** or **small change** and state the classification. When uncertain, use the large-change workflow. A task that begins small must be reclassified before it gains a second independently reviewable behavior change.

### Large changes

Use this workflow when the change spans components or public contracts, affects a trust boundary or migration, contains multiple independently testable behavior slices, or is expected to need multiple commits.

1. Start from the current integration branch and create a correctly named work branch.
2. Make the first meaningful, internally consistent commit, push the branch, and open a draft pull request. Do not create an empty commit solely to open a pull request.
3. Continue in reviewable behavior slices. Each commit must compile, preserve repository consistency, use a Conventional Commit subject, and include its directly owned tests or documentation.
4. Push commits without squashing or rewriting published history unless the user explicitly requests cleanup. Keep the pull-request description and validation evidence current.
5. When feedback arrives before merge, continue on the same branch and pull request with new focused commits. Do not open a replacement pull request.
6. Hand the completed pull request to the user for review and manual squash-merge. Do not merge it unless the user explicitly requests the merge.

### Small changes

Use this workflow only for one localized, coherent change that fits one commit and does not alter a trust boundary, public contract, migration, or release gate.

1. Prefer a short correctly named branch, one commit, and a pull request when review or CI visibility has value.
2. Commit directly to the current integration branch only when the user explicitly requests the direct path, the acting account may use the administrator bypass, the worktree contains no unrelated changes, and focused validation passes before the push.
3. Use one Conventional Commit. Do not split mechanical and documentation updates that are inseparable from the behavior, and do not combine unrelated cleanup.
4. If the task grows beyond one coherent commit, stop before committing, create a work branch, and continue under the large-change workflow.

The detailed branch, commit, review, and follow-up rules are authoritative in `.docs/code-review.md`.

## Code Quality and Security Standard

Before authoring or modifying code, contributors must read and follow `.docs/code-quality.md`. It defines the mandatory Google Go Style Guide rules, security requirements aligned with OWASP ASVS Level 2 and NIST SSDF, and performance rules for this repository. Code style, security controls, and performance constraints are not optional; they are enforced by the CI gates described in that document.

## Repository Structure

- `platform/client`: Go agent runtime and client transport.
- `platform/services/gateway`: Go gateway service, mTLS termination, and broker publication.
- `platform/shared`: shared protobuf schema and generated Go artifacts.
- `platform/website`: React and TypeScript administrative website. Browser input is operator-authored and is not identity or authorization evidence.
- `scripts`: operational helpers such as certificate tooling.
- `.docs`: operator-facing documentation and roadmap material.

## Development Entry Points

Use the repository `Makefile` as the primary developer interface.

- `make fmt`: format Go code across all modules.
- `make test`: run tests across all modules.
- `make tidy`: normalize module metadata across all modules.
- `make build`: build the gateway and client binaries into `bin/`.
- `make lint`: run `golangci-lint` across all modules.
- `make ci-go`: run the complete Go quality and cross-platform build gate.
- `make ci-web`: install the frozen Bun dependency graph, then check formatting, lint, types, and the production website build.
- `make ci`: run both language gates.
- `make clean`: remove build outputs.

## Change Workflow and Naming

- Branch from the current integration branch. Use `<type>/<short-kebab-case-scope>`, where `type` is `feature`, `fix`, `security`, `docs`, `refactor`, `test`, `build`, `ci`, or `chore`. The long-lived `rewrite` branch is an integration branch, not the naming model for new work.
- Use Conventional Commit subjects: `<type>(<scope>): <imperative summary>`. Keep one coherent behavior change per commit. Do not mix generated artifacts, dependency changes, and unrelated cleanup into a feature commit.
- Use lowercase `snake_case.go` only where Go requires multiple words, `kebab-case.md` for documentation, and `kebab-case.ts` or `kebab-case.tsx` for TypeScript and React files. React component and exported type names use `PascalCase`; functions, hooks, variables, and non-component files use `camelCase` identifiers.
- Generated files must be produced by their owning generator, must not be hand-edited, and must be committed only when the source contract changes.
- Pull requests must state component ownership, trust/data classification changes, validation evidence, migration or compatibility impact, and residual risk. Follow `.docs/code-review.md`.

## TypeScript and React Rules

- Bun and `platform/website/bun.lock` are the only website package-manager contract. Do not add npm, pnpm, or Yarn lockfiles.
- TypeScript strictness must remain enabled. Do not introduce `any`, unchecked type assertions, or untyped external response shapes; parse gateway responses into bounded, explicit types.
- Keep server state in TanStack Query, URL/navigation state in TanStack Router, and narrowly scoped client-only state in component state or Zustand. Do not duplicate server state in a global store.
- Components render and collect operator intent. They do not make trust decisions. Agent IDs, filesystem facts, terminal output, screen frames, and operation results displayed by the website remain untrusted unless the gateway explicitly authors the field.
- Encode untrusted text as text, never raw HTML. Do not use `dangerouslySetInnerHTML` for client- or operator-authored content.
- Hooks start with `use`; components remain focused; effects are reserved for synchronization with external systems. Prefer derived render state and event handlers over effect-driven state copies.
- Every state-changing control must expose pending, success, partial, and failure behavior, prevent accidental duplicate submission, and retain accessible labels and keyboard operation.
- New website behavior must pass `make ci-web`. Security-sensitive browser changes require gateway-side enforcement tests; a disabled or hidden control is not authorization.

## Documentation Authoring Standard

Documentation must be specific, formal, and implementation-aware. Treat it as a system description for system architects and system engineers.

- State what a component owns.
- State what it does not own.
- State the trust source for every identity assertion.
- State whether data is user-authored, client-authored, or server-authored.
- State operational constraints without hedging language.

## Guardrails

- Do not invent features that are not present in the codebase.
- Do not describe client payloads as trusted input.
- Do not weaken certificate validation or assume permissive transport behavior.
- Do not bypass the gateway for event ingestion.
- Do not expand documentation into procedural material for unauthorized use.

## Delivery Discipline

Automation and contributors should move decisively through a task and then exit. Over-analysis, speculative refactoring, and unrelated cleanup are not acceptable substitutes for completing the requested work.
