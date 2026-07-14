# CI, Build, and Release Contract

## Ownership

The root `Makefile` owns reproducible developer commands. `.github/workflows/ci.yml` owns pull-request, `main`, and `rewrite` verification. `.github/CODEOWNERS` owns review routing, and `.github/pull_request_template.md` owns the required change-evidence shape. Neither the repository nor workflow currently owns artifact publication, signing, deployment, rollback, or update distribution. Those release functions do not yet exist.

## Current CI Gates

The Go job installs pinned analysis tools and runs `make ci-go`. That target verifies formatting and module tidiness; runs unit, race, vet, Staticcheck, vulnerability, gosec, and golangci-lint analysis across the client, gateway, and shared modules; and cross-compiles gateway and client binaries for Linux, macOS, and Windows on `amd64` and `arm64`.

The website job installs pinned Bun `1.3.9` dependencies from `bun.lock` and runs `make ci-web`. That target checks Prettier formatting, ESLint, TypeScript, and a Vite production build. Bun is the only supported website package manager.

CI runs on pull requests, pushes to `main` and the temporary `rewrite` integration branch, and manual dispatch.

## Integration Branch Protection

The GitHub `rewrite` branch rule is enforced for administrators and contributors. It requires:

- the `Go quality and cross-platform build` and `Website quality and production build` checks on a branch current with its base;
- one code-owner approval, dismissal of stale approvals, and approval after the most recent push;
- resolution of review conversations; and
- rejection of force-pushes and branch deletion.

The default `main` branch remains outside the temporary rewrite-integration rule. Promotion to `main` is blocked by the Milestone 1 release decision and requires its own reviewed release-branch protection before merge.

Cross-compilation proves compilation only. It does not prove filesystem safety, process behavior, screen capture, terminal execution, packaging, installation, or upgrade behavior on a target operating system.

## Known CI Gaps Before Milestone 1

- No browser unit, component, end-to-end, accessibility, or visual-regression harness exists.
- No native operating-system integration runners exercise Linux, macOS, and Windows behavior.
- No protobuf generation or schema-compatibility check proves generated artifacts match `events.proto`.
- No secret scan, dependency-license policy, SBOM, provenance attestation, binary signing, checksum manifest, or reproducibility comparison exists.
- No container/deployment configuration validation exists. Development compose files use floating images and development credentials and are not release inputs.
- No integration environment exercises mTLS enrollment, command rotation, NATS security, gateway restart, transfer recovery, or failure injection.
- No coverage threshold or fuzzing job is enforced.
- GitHub Actions are referenced by version tags rather than immutable commit SHAs. Pin action revisions before production release.

## Release Pipeline Required Before Publication

Release automation must remain separate from pull-request CI and must run only from a protected, signed version tag after all milestone gates pass. It must:

1. Build from a clean checkout with pinned Go, Bun, analysis tools, actions, and future native toolchains.
2. run all CI, platform integration, security regression, recovery, accessibility, and performance gates;
3. generate version metadata, dependency inventories, SBOMs, and a release evidence record;
4. build each supported artifact once, publish SHA-256 checksums, sign artifacts and provenance with protected release identity, and verify signatures before publication;
5. package only documented runtime files and exclude development credentials, state, test data, and local configuration;
6. stage artifacts in a non-production environment and run install, upgrade, rollback, and command-key transition tests; and
7. require explicit release approval from product, gateway-security, and operations owners.

The repository must not publish a release while `.docs/project-status.md` reports `NOT READY`.

## Versioning and Branch Flow

Use semantic versions after Milestone 1 establishes a stable supported contract. Until then, builds are development snapshots and must not carry production or compliance claims. Work branches follow `.docs/code-review.md`; `rewrite` remains the temporary integration branch for the Go rewrite. Merge it into a protected release branch only after the milestone acceptance record is complete. Do not maintain competing Python and Go top-level products: historical Python behavior is not a release contract unless explicitly reintroduced through an approved plan.
