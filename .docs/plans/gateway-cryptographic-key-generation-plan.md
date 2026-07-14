# Gateway Cryptographic Key-Generation Plan

> **Standards correction:** As of July 2026, NIST has not published SP
> 800-90A Revision 3. The current final is SP 800-90A Revision 1, while
> Revision 2 is only pre-draft work. The implementation shall conform to the
> current final publication and adopt its successor after finalization and
> validation. Claiming compliance with a nonexistent Revision 3 is prohibited.
> See [NIST SP 800-90A Revision 1][sp-800-90a] and the
> [Revision 2 pre-draft notice][sp-800-90a-r2].

An approved algorithm is not automatically a FIPS 140-3 validated
implementation. Production approval depends on the exact cryptographic module
certificate, module version, operating environment, approved mode, and security
policy.

## Implementation status

Status date: 2026-07-14.

The current implementation establishes the gateway-owned software-provider
boundary and removes command-signing reuse of the TLS identity. It does not
constitute production approval of a deployment. Production approval still
depends on the selected provider certificate, covered operating environment,
provider security policy, operational controls, and the controls listed as
missing below.

For Milestone 1, every cryptographic service exposed by the runtime must close
its applicable partial and missing controls and produce the test and approval
evidence in this plan. A service that is not required for the bounded milestone
must remain disabled at the public contract; dormant design text is not a
reason to expose a partially protected algorithm. The release decision is
tracked in `.docs/project-status.md` and `.docs/roadmap.md`.

### Completed production logic

- `platform/services/gateway/internal/keyservice` owns software-provider
  validation, key generation, key metadata, lifecycle state, and fail-closed
  readiness. It does not own agent authentication, authorization, protobuf
  validation, or distributed nonce storage.
- Gateway startup requires FIPS mode, reads the exact frozen Go module identity
  from build information, maps `v1.0.0-c2097c7c` to CMVP certificate #5247 and
  its pinned security-policy identity, and checks the deployment-provided
  `GOOS/GOARCH` allowlist. A mismatch prevents key generation and prevents the
  gateway from connecting to NATS.
- Startup runs non-secret consistency probes for the random source,
  RSA-PSS sign/verify, P-384 agreement, ML-KEM-768 encapsulation/decapsulation,
  and AES-256-GCM seal/open. Probe failures place the key service in a
  fail-closed error state.
- Application-visible key generation uses `crypto/rand`. The key service does
  not replace `crypto/rand.Reader` and does not implement an application-level
  entropy mixer.
- AES-256-GCM DEK generation is implemented with an opaque AEAD handle. A DEK
  is bound to one purpose, security domain, and traffic direction. Callers must
  provide risk-derived invocation, byte, and rotation limits no greater than
  the 24-hour and 2^32 invocation fail-safe ceilings.
- AES-GCM sealing cannot accept a caller-selected nonce. It requires a narrow
  strongly-consistent nonce allocator interface. Allocation failure,
  uncertainty, an invalid nonce length, expiration, or an operating-limit
  breach retires the affected DEK before encryption.
- AES-GCM authenticated context is a typed canonical structure binding protocol
  version, gateway, sender, recipient, security domain, session, sequence,
  event, issuance and expiry, direction, operation, key ID, and algorithm.
- Ephemeral P-384 and ML-KEM-768 private keys are represented by opaque
  one-use handles. Peer P-384 keys are parsed by the standard library. Raw
  agreement secrets do not leave the handle; HKDF-SHA-384 derives a bounded,
  context-bound traffic key, clears the temporary shared-secret slice, and
  destroys the ephemeral private handle.
- RSA-PSS command signing no longer loads `server.key`. The gateway generates
  or loads a dedicated RSA-3072 PKCS #8 key from `command-signing.key`, requires
  owner-only file permissions, bounds and validates the key file, and rejects
  command-key paths that overlap TLS artifacts.
- The command verification key is published atomically as
  `command-signing.pub` before the signing key transitions from `preactive` to
  `active`. The client loads this independent public key and enforces a minimum
  RSA size. The command queue consumes only an opaque signing capability and
  key ID.
- The key lifecycle model defines `preactive`, `active`, `verify-only`,
  `retired`, `revoked`, and `destroyed`. Command-signing and DEK handles enforce
  the transitions applicable to their current use. Command signing stops at
  the 90-day validity limit and causes readiness to fail.
- Administrative liveness and readiness endpoints are separate. Readiness
  reflects provider state and command-signing-key availability without
  returning provider or key details to the caller.
- Gateway, dashboard, and client TLS remain TLS 1.3 and currently pin P-384.
  No application-defined hybrid wire format or unapproved TLS hybrid group was
  introduced.

### Partially implemented controls

- The DEK API enforces nonce allocation and usage contracts, but no concrete
  strongly consistent prefix-and-counter store is implemented. Consequently,
  application event encryption is not enabled.
- Command-key lifecycle transitions and public-key prepublication are
  implemented, but automated multi-key rotation, durable key-version metadata,
  client refresh, and bounded verification-only overlap are not implemented.
- The software provider exposes opaque handles to consumers, but its dedicated
  command key is persisted as an owner-only PKCS #8 secret file. Non-exportable
  HSM/KMS command keys are not implemented.
- Exact Go module, certificate, security-policy, and `GOOS/GOARCH` checks are
  implemented. Deployment evidence that the complete operating environment is
  covered by the certificate remains an external promotion gate.
- Provider failures block startup and readiness, but a persistent bounded
  security-alarm sink, recovery state machine, and restart-rate limiting are
  not implemented.

### Missing implementation

- PKCS #11 and cloud KMS providers, authenticated provider sessions,
  workload-identity authorization, non-exportable key generation, KEK-backed
  envelope wrapping, provider logout, and provider destruction evidence.
- A strongly consistent distributed nonce store implementing random prefix
  registration, durable counter-range leases, process-incarnation binding,
  allocation epochs, rollback detection, quorum-loss handling, and mandatory
  DEK retirement.
- DEK persistence through authenticated wrapping, multi-instance key managers,
  risk-profile configuration, scheduled rotation, staged activation,
  verification-only overlap, revocation propagation, and destruction across
  replicas and backups.
- Versioned encrypted or attested protobuf fields, gateway event-protection
  integration, durable replay and idempotency records, per-session sequence
  windows, and broker retry preservation for authenticated operations.
- ML-DSA-65 binary-attestation signing and canonical attestation manifests. The
  Go 1.26 module remains unsuitable for a production approval claim until an
  applicable CMVP certificate covers the selected build and environment; an
  approved HSM/KMS provider is also not integrated.
- Approved library-owned hybrid TLS configuration. The current implementation
  deliberately uses P-384 only and does not claim that the available Go hybrid
  groups are covered by certificate #5247.
- Independent NATS mutual-TLS credentials and TLS provider configuration.
- The workload directory and operation-, tenant-, session-, agent-, message-,
  subject-, and command-queue authorization layer described by this plan.
- Signed cryptographic policy and provider allowlist artifacts, monotonic
  policy and key-metadata rollback protection, quorum administrative approval,
  immutable audit storage, separation-of-duties enforcement, and cryptographic
  inventory reconciliation.
- HSM/KMS concurrency limits, per-identity quotas, circuit breaking, provider
  capacity alarms, and denial-of-service controls at cryptographic boundaries.
- Tenant KEK/DEK namespaces, tenant nonce and audit partitions, recoverable-key
  backup and restore, cryptographic erasure, and deletion across snapshots and
  provider-retained versions.
- The test plan, fault injection, certification-evidence automation, and the
  isolated sandbox. These were intentionally excluded from the current
  implementation scope and remain required before production promotion.

## Threat model

The gateway is the sole trust boundary. It authenticates agents through TLS 1.3
mutual TLS, derives agent identity from the verified client certificate, authors
security metadata, and publishes events to the broker.

The design protects against:

- Prediction or reconstruction of gateway-generated keys after partial entropy
  source failure.
- Hardware RNG, operating-system RNG, virtual-machine cloning, early-boot, or
  DRBG-state failures.
- Private-key extraction through logs, crash dumps, heap retention, debugging,
  or configuration files.
- Nonce reuse under AES-GCM.
- Key substitution, rollback, stale-key use, and confused-deputy failures during
  rotation.
- Compromise of one key domain affecting TLS, broker transport, event
  protection, command signing, or binary attestation.
- A malicious client attempting to assert its own identity, key ID,
  authentication status, timestamp, or attestation result.
- Harvest-now-decrypt-later attacks against stored encrypted traffic.
- Replay, reordering, and duplication of authenticated messages.
- Rollback of binaries, configuration, cryptographic policy, provider
  allowlists, or key metadata.
- Exhaustion of HSM/KMS capacity and side-channel leakage at cryptographic
  boundaries.

The design does not treat client-authored protobuf fields, claimed public keys,
binary hashes, or attestation results as trusted. Trust begins only after mutual
TLS certificate validation and gateway-side verification.

Key domains shall be independent:

1. Gateway TLS identity.
2. NATS client identity.
3. Command signing.
4. Binary-attestation signing.
5. Data-encryption keys.
6. ECDH and ML-KEM key-establishment material.
7. HSM/KMS wrapping keys.

The existing loading of `server.key` as the command-signing key in
`platform/services/gateway/cmd/runtime.go` shall be retired. A TLS private key
must not double as a command or attestation key.

## Entropy sourcing

### Approved architecture

Production key generation shall occur inside one of:

- A FIPS 140-3 validated HSM/KMS in its approved mode, with a documented SP
  800-90A/B/C random-bit generator.
- The CMVP-validated Go Cryptographic Module on a certificate-covered operating
  environment when an HSM cannot implement the required algorithm.

The random-bit generator shall follow an approved SP 800-90C construction
combining:

- A credited entropy source assessed under SP 800-90B.
- CTR_DRBG, HMAC_DRBG, or Hash_DRBG under the current final SP 800-90A.
- Approved conditioning and reseeding rules.

SP 800-90C is the governing construction standard connecting 90B entropy
sources to 90A DRBGs. See [NIST SP 800-90B][sp-800-90b] and
[NIST SP 800-90C][sp-800-90c].

### Source combination

Use at least two independent inputs where the selected validated module's
security policy permits:

- **Primary credited source:** HSM noise source, validated CPU-jitter source,
  TPM entropy source, or RDSEED-backed entropy source with an applicable
  ESV/CMVP claim.
- **Additional uncredited input:** Linux `getrandom(2)`, with `/dev/urandom`
  only through the operating-system CSPRNG fallback, and, where independently
  available, TPM/HSM random output.

RDRAND is the output of a hardware DRBG, not raw noise; RDSEED is intended for
seeding. Neither shall be read directly by gateway application code or assumed
healthy merely because the instruction succeeds.

Mixing shall occur inside the validated cryptographic boundary using the
construction and approved conditioning function named in the module security
policy. Sources shall be domain-separated and supplied as entropy input, nonce,
personalization string, or additional input only as that construction permits.
Application-written XOR, concatenation, hashing, HKDF-based entropy pools, or
entropy-credit addition is prohibited.

The Go FIPS module's approved mode implements `crypto/rand.Reader` using an SP
800-90A DRBG and mixes platform CSPRNG output as uncredited additional data.
Newer module versions also document an ESV-certified CPU-jitter source. See
[Go FIPS 140-3 compliance][go-fips].

### Health monitoring and failure behavior

The credited raw noise source shall run SP 800-90B startup and continuous
health tests before conditioning:

- Repetition Count Test.
- Adaptive Proportion Test.
- Vendor-specific tests justified by the source failure analysis.
- Power-up self-tests, DRBG known-answer tests, and key-pair consistency tests
  required by the validated module.

Any health-test failure, self-test failure, unavailable credited source, reseed
failure, repeated hardware-instruction failure, or loss of approved-mode status
shall place the crypto provider into an error state. The gateway shall:

1. Stop generating keys and establishing new sessions.
2. Refuse readiness and broker publication where protection depends on new key
   material.
3. Preserve no partial keys or entropy samples.
4. Emit a bounded security alarm containing provider identity and error code,
   never entropy or key bytes.
5. Require provider recovery and successful startup tests before re-entering
   service.

Restart loops must be rate-limited. Restarting is not a substitute for
investigating a repeated health failure.

## Key types & generation

| Purpose | Required construction | Generation and use |
| --- | --- | --- |
| Event/data protection | AES-256-GCM, SP 800-38D | Generate a uniform 256-bit DEK inside the HSM/KMS or from approved `crypto/rand`. Use the distributed 96-bit nonce construction defined below. Bind protocol version, gateway identity, agent ID, session ID, event ID, key ID, direction, tenant/security domain, and algorithm as AAD. Never reuse a key/nonce pair. |
| Classical agreement | ECDH P-384, SP 800-56A Revision 3 | Generate an ephemeral P-384 key per TLS/session agreement. Validate peer public keys using the approved implementation. Derive traffic keys using an approved SP 800-56C Revision 2 KDF with explicit context and role binding. |
| PQC agreement | ML-KEM-768, FIPS 203 | Prefer ephemeral ML-KEM-768 decapsulation keys. Use the standardized FIPS 203 encoding and reject malformed public keys and ciphertexts. ML-KEM shared secrets must feed an approved KDF; they are not directly reused as multiple application keys. |
| Hybrid transport agreement | Go `crypto/tls` supported hybrid group | Use only a library-defined TLS 1.3 group covered by the selected frozen Go module and its CMVP certificate. Go 1.26 defines `SecP256r1MLKEM768` and `SecP384r1MLKEM1024`; it also supports `X25519MLKEM768`. Do not reproduce their wire format or combiner in gateway application code. |
| Binary attestation | ML-DSA-65, FIPS 204 | Generate and retain the signing key in an HSM/KMS supporting ML-DSA in approved mode. Sign a canonical manifest containing artifact digest, build identity, version, target, issuance time, policy version, and key ID. |
| Compatibility attestation | Ed25519/EdDSA only when explicitly covered | Ed25519 is standardized through FIPS 186-5, but it may only be used when the selected module certificate and approved-mode security policy explicitly cover the exact EdDSA service. Prefer ML-DSA-65 for the new attestation profile. |

For transport, TLS owns the hybrid wire encoding, transcript binding,
initiator/responder roles, downgrade protection, KDF labels and inputs, peer
authentication relationship, alerts, and failure behavior. Configure
`tls.Config.CurvePreferences` to the groups covered by the selected frozen
module and deployment certificate; do not infer coverage from availability in
the current Go source tree. The gateway shall abort the handshake rather than
silently negotiate an unapproved group when policy requires hybrid transport.
Go 1.26 enables the NIST-curve hybrid groups in the standard TLS library, but
the corresponding frozen Go Cryptographic Module v1.26.0 remains pending CMVP
review as of this plan. See the [Go 1.26 TLS release notes][go-126] and
[Go FIPS module status][go-fips].

An application-level P-384 plus ML-KEM protocol is out of scope for direct
implementation. It may be proposed only in a separate, versioned protocol
specification that fixes:

- Exact wire encoding, length bounds, canonical parsing, and transcript.
- Initiator and responder roles and state transitions.
- Algorithm negotiation and downgrade protection.
- KDF construction, ordered inputs, salt, labels, output partitioning, and
  domain separation.
- Authentication and key-confirmation relationship to mutual TLS.
- Failure behavior that does not disclose which component failed.
- Contributory behavior and the result when either classical or PQC input is
  invalid or adversarially controlled.
- Replay and reordering protection.
- Certificate, CAVP, and CMVP coverage for the complete service.

That specification requires independent cryptographic review, interoperability
vectors, and approval of applicable validation coverage before implementation.
Raw P-384 and ML-KEM shared secrets shall never be concatenated and used as a
key.

ML-KEM and ML-DSA are standardized by [FIPS 203][fips-203] and
[FIPS 204][fips-204]. Their published errata shall be tracked in the
cryptographic baseline.

## Storage & lifecycle

- Generate non-exportable private keys inside the HSM/KMS whenever the provider
  supports the required approved service.
- Represent private keys in gateway code as opaque handles implementing narrow
  signing, decapsulation, or key-unwrapping interfaces.
- Store only public keys, key IDs, provider references, lifecycle state, and
  wrapped key blobs outside the provider.
- Use envelope encryption: HSM/KMS KEKs wrap short-lived AES DEKs. The gateway
  must never persist plaintext DEKs.
- Disable key export, backup, or cloning unless performed through the
  provider's approved authenticated wrapping mechanism.
- Exclude keys, shared secrets, entropy samples, plaintext manifests, and
  ciphertext-decapsulation failures from logs and metrics.
- Disable core dumps, restrict debugger and `ptrace` access, prevent secrets
  from entering environment variables, and keep crash reports free of memory
  snapshots.
- Avoid converting secrets to strings or retaining them in pooled buffers.
  Zero temporary byte slices immediately after use, while recognizing that Go
  garbage collection makes universal zeroization unverifiable. HSM-backed
  opaque keys are therefore the production preference.
- On shutdown, stop accepting work, drain operations, destroy ephemeral
  provider objects, zero owned secret buffers, close authenticated HSM
  sessions, and confirm provider logout.

### Distributed AES-GCM nonce allocation

Each encryption direction and tenant or security domain shall use a distinct
DEK. A DEK shall not be shared between gateway-to-agent, agent-to-gateway,
broker, tenant, or unrelated protocol contexts.

For each DEK, construct the 96-bit nonce as:

```text
nonce = 32-bit instance prefix || 64-bit monotonically increasing counter
```

The design shall enforce all of the following:

- A strongly consistent store atomically registers a random 32-bit prefix as
  unique within the DEK. Prefix generation uses the approved CSPRNG; a
  collision causes regeneration, never reuse.
- The store atomically leases non-overlapping counter ranges for that
  `(DEK ID, instance prefix, process incarnation)` and durably advances the
  high-water mark before returning a range. The gateway may consume a leased
  range locally but shall never reuse unused values after restart.
- Every process incarnation obtains a new registered prefix. Instance names,
  IP addresses, timestamps, restored local state, and VM identifiers are not
  nonce prefixes.
- Store records bind the DEK ID, key version, tenant/security domain,
  direction, instance prefix, counter range, allocation epoch, and integrity
  metadata.
- Restored snapshots cannot roll back the authoritative high-water mark or
  allocation epoch. Startup compares local metadata with the authoritative
  store before encryption.
- Loss of allocation quorum, duplicate prefix/range detection, rollback,
  conflicting metadata, uncertain local state, or exhaustion immediately
  disables encryption and retires the affected DEK. The gateway generates a
  new DEK rather than guessing the next nonce.

The per-key invocation ceiling is a fail-safe, not an operating target. The
cryptographic owner shall derive a substantially lower rotation threshold from
maximum message size, aggregate blocks processed, expected message volume,
authentication-tag length, acceptable forgery probability, number of gateway
instances, and the applicable SP 800-38D analysis. The key manager shall enforce
the lowest of the risk-derived invocation limit, data-volume limit, time limit,
or counter-space limit.

Rotation policy:

- **AES-256-GCM DEKs:** rotate every 24 hours or earlier at the documented,
  risk-derived message, block, byte, or forgery-probability limit. Enforce an
  absolute fail-safe ceiling of 2^32 invocations per key and stop well before
  that ceiling or nonce exhaustion.
- **ECDH/ML-KEM ephemeral keys:** one session or handshake; destroy immediately
  afterward.
- **Long-lived ML-KEM recipient keys, if operationally required:** rotate at
  most every 30 days with bounded overlap.
- **Attestation and command-signing keys:** rotate every 90 days, with staged
  publication of the new public key before activation and a bounded
  verification-only overlap.
- **KEKs and TLS identities:** rotate at least annually or according to the
  shorter provider/PKI policy.
- **All keys:** rotate immediately after suspected exposure, provider
  compromise, relevant cryptographic errata, personnel/control-plane
  compromise, or failed provenance checks.

Key states shall be `preactive`, `active`, `verify-only`, `retired`, `revoked`,
and `destroyed`. Encryption and signing may use only `active` keys. Decryption
and verification may use a bounded set of `active` and `verify-only` keys.

### Administrative and operational controls

- Separate key custodians, platform operators, security approvers, and
  auditors. Creation or import of trust anchors, policy changes, key export,
  recovery, revocation, deletion, and emergency rotation require quorum
  approval and leave an immutable audit record.
- Give each gateway workload a short-lived, revocable workload identity. HSM
  and KMS policy shall allow that identity only the named operations on its
  assigned keys; deny wildcard key access, administrative operations, and key
  export.
- Protect audit records with append-only or write-once retention,
  integrity-protected timestamps, authenticated actor and workload identity,
  operation, key ID/version, decision, and trace ID. Never record secret
  material.
- Enforce certificate revocation and short certificate lifetimes for gateway,
  agent, broker, and control-plane identities. Define fail-closed behavior for
  unavailable revocation status where policy requires online checking.
- Sign the cryptographic-policy configuration and provider allowlist. Pin a
  monotonic policy version and verified artifact digest so binaries,
  configuration, provider identities, and key metadata cannot be rolled back.
- Apply bounded concurrency, request quotas, backoff, circuit breaking, and
  per-identity rate limits to prevent KMS/HSM exhaustion. Capacity failure must
  not trigger a software-key fallback.
- Review secret-dependent timing, cache behavior, error distinctions, shared
  HSM tenancy, and remote timing at every cryptographic boundary. Use only
  provider implementations covered by the applicable side-channel claims.
- Maintain a machine-readable inventory of algorithms, protocols, keys,
  certificates, module certificates, operating environments, dependencies,
  owners, consumers, expiry dates, and deprecation status.
- Implement policy-driven algorithm deprecation with `allowed`, `verify-only`,
  and `forbidden` states. New use stops before legacy verification is removed.
- Back up only keys whose availability requirements demand recovery, using the
  provider's approved authenticated wrapping and split control. Test restore,
  regional disaster recovery, access revocation, and audit continuity in the
  sandbox.
- Define deletion across active replicas, caches, wrapped backups, snapshots,
  disaster-recovery copies, and provider-retained key versions. Record provider
  destruction evidence and retention exceptions.
- Separate tenant KEKs and DEKs, authorization namespaces, nonce allocation,
  audit partitions, and quotas. Cryptographic erasure destroys the tenant's
  wrapping keys and all recoverable replicas under an approved deletion
  procedure.

## Implementation (Go)

Create an internal gateway-owned crypto subsystem, for example
`platform/services/gateway/internal/keyservice`. It owns provider selection,
approved-mode checks, generation, rotation, key IDs, and fail-closed readiness.
It does not own agent identity or protobuf validation.

Use:

- `crypto/rand` as the sole application-visible random source.
- `crypto/aes` and `cipher.NewGCM` only through a certificate-covered FIPS
  module.
- `crypto/ecdh.P384().GenerateKey(rand.Reader)` for P-384.
- `crypto/mlkem` and `mlkem.GenerateKey768()` for FIPS 203 ML-KEM.
- `crypto/mldsa` for FIPS 204 ML-DSA when using Go module v1.26.0 or later.
- PKCS #11 or the cloud KMS SDK only when the provider/module certificate
  covers the requested service.

The repository currently targets Go 1.25.12. ML-DSA requires a toolchain/module
upgrade, and the Go v1.26.0 cryptographic module is presently listed as pending
rather than CMVP certified. Production ML-DSA must therefore use a validated
HSM/KMS or wait for an applicable module certificate. The Go v1.0.0 module is
CMVP certificate #5247; build selection can use `GOFIPS140=certified`. See the
[Go module status][go-fips] and [`crypto/mldsa` documentation][go-mldsa].

`filippo.io/mlkem768` may be used only for sandbox interoperability if
required. It does not create a FIPS validation claim. Prefer standard-library
[`crypto/mlkem`][go-mlkem] in production.

At startup, the gateway shall:

1. Verify `crypto/fips140.Enabled()` and record `crypto/fips140.Version()`.
2. Verify the exact approved provider, certificate, operating environment, and
   security-policy version against an allowlist.
3. Execute a non-secret provider self-test and key-generation/sign/verify
   consistency probe.
4. Remain unready on any mismatch.

`GODEBUG=fips140=only` is suitable for CI diagnostics, not production; Go
documents it as best-effort and potentially panic-inducing. Production shall
select the certified module at build time.

Custom RNGs, replacement of `crypto/rand.Reader`, deterministic test readers in
production, `math/rand`, timestamps, UUIDs, counters, process IDs, MAC
addresses, passwords, or hashes of such values as entropy are explicitly
forbidden.

### Gateway and protobuf integration

The mutual TLS configuration in
`platform/services/gateway/internal/transport/http.go` remains the
authentication boundary. Cryptographic agility must not permit a client to
bypass `tls.RequireAndVerifyClientCert` or TLS 1.3.

Authentication establishes a certificate-bound identity; it does not grant
authority. After mutual TLS authentication and before parsing expensive
cryptographic objects or publishing data, the gateway shall map the derived
identity to an active workload record and enforce authorization for the exact
agent, tenant/security domain, session, operation, message type, broker subject,
and command queue. A valid certificate with no matching authorization is
denied. Authorization decisions are server-authored and audited.

Before cryptographic processing, enforce bounded request bodies, protobuf
message sizes, field counts, nesting depth, algorithm allowlists, exact key,
ciphertext, and signature lengths, canonical encodings, and per-identity request
rates. Invalid data is rejected before HSM/KMS calls where possible, without
creating distinguishable cryptographic error oracles.

The gateway shall continue to overwrite the server-authored fields in
`platform/shared/proto/events.proto` before broker publication. If encrypted or
attested events are introduced, make an additive, versioned protobuf change
containing only:

- Algorithm identifier from a closed enum.
- Key ID and key version.
- Nonce, ciphertext and tag, or signature.
- Public attestation metadata.

Private keys and shared secrets never enter protobufs. `agent_id`, `session_id`,
`event_id`, gateway timestamp, authentication result, and verified attestation
result remain gateway-authored. Client-provided equivalents are untrusted data.

Every protected application message shall bind a protocol version,
authenticated sender and intended recipient, tenant/security domain, session
ID, direction, monotonic per-session sequence number, unique message/event ID,
issuance time, expiry, operation, and key ID into its signed content or AEAD
associated data. The gateway maintains a bounded replay window and durable
idempotency record at the trust boundary. It rejects duplicates, expired
messages, sequence rollback, cross-session reuse, wrong-direction messages, and
reordering where the operation requires strict order. Broker retries preserve
the original event ID and do not create a new authenticated operation.

NATS publication continues only through the gateway. Broker transport shall use
independently issued mutual TLS credentials; event-encryption keys must not be
reused as NATS credentials.

## Compliance mapping (FIPS 140-3 / SP 800-90A/B/C)

| Requirement | Design control |
| --- | --- |
| FIPS 140-3 | Exact CMVP-certified module, approved mode, covered operating environment, self-tests, pairwise consistency tests, non-exportable keys, and provider security policy. |
| SP 800-90A | Validated CTR_DRBG, HMAC_DRBG, or Hash_DRBG. Current final is Revision 1; migrate after a successor becomes final and validated. |
| SP 800-90B | Assessed entropy source, min-entropy claim, startup tests, Repetition Count Test, Adaptive Proportion Test, continuous monitoring, and fail-closed response. |
| SP 800-90C | Approved composition of the 90B entropy source and 90A DRBG; auxiliary OS/hardware inputs receive no undocumented entropy credit. |
| SP 800-38D | AES-256-GCM, unique 96-bit nonces, invocation limits, authenticated metadata, and fail-before-exhaustion behavior. |
| SP 800-56A/C | P-384 key agreement, public-key validation, approved extraction/expansion, and protocol-context binding. |
| FIPS 203 | ML-KEM-768 key generation, encapsulation, decapsulation, standardized encoding, and applicable errata. |
| FIPS 204 | ML-DSA-65 attestation signatures, canonical signed content, context separation, and applicable errata. |
| FIPS 186-5 | EdDSA only where the selected module's validation explicitly covers the service. |
| TLS hybrid transport | Standard-library TLS group only; transcript, negotiation, downgrade protection, combiner, and failure behavior remain inside the covered TLS service. |

## Test plan

### Entropy and provider tests

- Unit-test SP 800-90B Repetition Count and Adaptive Proportion logic against
  passing, stuck-bit, repeated-symbol, biased-window, boundary, and simulated
  source-loss sequences.
- Verify that every injected health-test, reseed, provider-session, and
  self-test failure makes readiness fail and prevents key generation.
- Capture raw-source samples only in an isolated laboratory build, never from
  production or after conditioning.
- Confirm deterministic test sources cannot be linked into production builds.

Provider certificate status, the SP 800-90B entropy assessment, module security
policy, covered operating environment, approved-mode configuration, health-test
integration, and fail-closed behavior are mandatory acceptance gates.

#### Optional laboratory diagnostics

NIST STS and `dieharder` may be run over large, isolated laboratory samples
only when investigating a source or regression. Their output is not a release,
promotion, entropy-quality, or compliance gate. Passing results do not
establish min-entropy or correct integration; failing results trigger
investigation but do not replace the SP 800-90B source assessment and health
tests.

### Cryptographic correctness

- Run NIST/CAVP known-answer vectors for AES-GCM, P-384, ML-KEM, and ML-DSA.
- Test malformed P-384 points, ML-KEM keys and ciphertexts, signatures, nonces,
  key IDs, and algorithm identifiers.
- Test AES-GCM nonce uniqueness across restart, failover, concurrency, and
  counter-store recovery. Cover range-allocation races, prefix collision,
  exhausted ranges, loss of quorum, restored-snapshot rollback, uncertain
  high-water marks, and mandatory DEK retirement.
- Verify domain separation and that changing any authenticated protobuf field
  invalidates decryption or attestation.
- Test the configured TLS hybrid groups against supported and unsupported
  peers, downgrade attempts, HelloRetryRequest, malformed shares, certificate
  failure, and policy requiring hybrid-only negotiation. Verify the negotiated
  service is covered by the selected frozen module and certificate.
- If an application hybrid protocol is ever approved, require independent
  interoperability vectors for every transcript, role, negotiation, failure,
  key-confirmation, and contributory-behavior case before gateway integration.

### FIPS verification

- Build and test with `GOFIPS140=certified`.
- Record `go version -m`, module version, artifact digest, CMVP certificate, and
  operating-environment evidence.
- Run CI with FIPS mode enabled and a separate diagnostic job using
  `GODEBUG=fips140=only`.
- Assert startup refusal when FIPS mode is off, the module version is
  unapproved, or a provider reports non-approved mode.
- Revalidate the complete artifact whenever the Go toolchain, module, compiler
  flags, HSM firmware, operating system, CPU architecture, or cryptographic
  dependency changes.

### Rotation integration tests

- Prepublish a new public key, activate it atomically, retain the previous key
  for verification, then retire and destroy it.
- Test concurrent in-flight events across the activation boundary.
- Test rollback, unknown key IDs, revoked keys, premature activation, missed
  rotations, HSM outage, and multi-instance races.
- Confirm clients cannot select a signing key, alter gateway-authored metadata,
  or cause publication under a retired key.
- Verify that gateway mutual TLS authentication and protobuf provenance remain
  unchanged throughout rotation.

### Authorization and operational tests

- Verify that authentication without operation-, tenant-, session-, agent-,
  and broker-subject authorization is denied before publication or command
  access.
- Test bounded parsing and rejection of oversized, deeply nested, malformed,
  noncanonical, unknown-algorithm, and high-rate cryptographic inputs before
  provider exhaustion.
- Test duplicate, expired, reordered, wrong-direction, cross-session, and
  sequence-rollback messages, including broker redelivery and gateway failover.
- Exercise quorum approval, separation of duties, immutable audit
  verification, short-lived identity expiry, certificate revocation, rate
  limiting, and denial of software fallback during HSM/KMS failure.
- Attempt rollback of the binary, signed crypto policy, provider allowlist, key
  metadata, nonce-allocation epoch, and algorithm-deprecation state.
- Test recoverable-key backup and disaster restoration under split control,
  and verify deletion across replicas, backups, snapshots, caches, and
  provider-retained versions.
- Run tenant-isolation and cryptographic-erasure tests proving that one tenant
  cannot address, unwrap, exhaust, replay into, or audit another tenant's keys.
- Perform dependency-inventory reconciliation and a timing/side-channel review
  before each production promotion and after provider, firmware, toolchain, or
  architecture changes.

## Sandbox

Build an isolated sandbox containing:

- A gateway instance, test agent, TLS 1.3 mutual TLS PKI, NATS instance, and a
  separate key-management namespace.
- An HSM/KMS test partition where available. A software HSM may support
  functional tests but conveys no production FIPS claim.
- Synthetic SP 800-90B noise-source adapters compiled only under test build
  tags.
- Fault injection for stuck sources, biased samples, DRBG reseed failure, HSM
  logout, key revocation, stale caches, restart, and partial rotation.
- No production certificates, keys, entropy captures, broker subjects, or
  credentials.
- Network isolation and audit capture sufficient to demonstrate that no client
  bypasses the gateway.

Promotion requires successful sandbox rotation, fail-closed, restart,
interoperability, and provenance tests plus approval of the exact CMVP/ESV
certificate set. No algorithm may move to production on the strength of library
reputation, CAVP vectors, NIST STS, or `dieharder` results alone.

[fips-203]: https://csrc.nist.gov/pubs/fips/203/final
[fips-204]: https://csrc.nist.gov/pubs/fips/204/final
[go-fips]: https://go.dev/doc/security/fips140
[go-126]: https://go.dev/doc/go1.26
[go-mldsa]: https://pkg.go.dev/crypto/mldsa
[go-mlkem]: https://pkg.go.dev/crypto/mlkem
[sp-800-90a]: https://csrc.nist.gov/pubs/sp/800/90/a/r1/final
[sp-800-90a-r2]: https://csrc.nist.gov/pubs/sp/800/90/a/r2/iprd
[sp-800-90b]: https://csrc.nist.gov/pubs/sp/800/90/b/final
[sp-800-90c]: https://csrc.nist.gov/pubs/sp/800/90/c/final
