# AGENTS.md

## Mission

This repository implements an internal remote screening platform composed of an agent, a gateway, and a shared event schema. Work in this tree must preserve the integrity of the trust boundary at the gateway and must maintain the repository as a controlled, authorized system.

## Mandatory Working Pattern

1. Identify the owning component before editing.
2. Verify the local runtime contract before changing a public interface.
3. Prefer the smallest coherent edit that preserves behavior.
4. Validate the touched slice immediately after the first substantive change.
5. Stop only after the repository state is internally consistent and the requested outcome is complete.

## Code Quality and Security Standard

Before authoring or modifying code, contributors must read and follow `.docs/code-quality.md`. It defines the mandatory Google Go Style Guide rules, security requirements aligned with OWASP ASVS Level 2 and NIST SSDF, and performance rules for this repository. Code style, security controls, and performance constraints are not optional; they are enforced by the CI gates described in that document.

## Repository Structure

- `platform/client`: Go agent runtime and client transport.
- `platform/services/gateway`: Go gateway service, mTLS termination, and broker publication.
- `platform/shared`: shared protobuf schema and generated Go artifacts.
- `scripts`: operational helpers such as certificate tooling.
- `.docs`: operator-facing documentation and roadmap material.

## Development Entry Points

Use the repository `Makefile` as the primary developer interface.

- `make fmt`: format Go code across all modules.
- `make test`: run tests across all modules.
- `make tidy`: normalize module metadata across all modules.
- `make build`: build the gateway and client binaries into `bin/`.
- `make lint`: run `golangci-lint` across all modules.
- `make clean`: remove build outputs.

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
