---
title: Common Criteria Evaluation
status: exploration
created: 2026-07-14
last_updated: 2026-07-14

---

# Common Criteria Evaluation

## Exploratory and Non-Binding Status

This document records a repository-informed investigation. It is not an
approved feature, implementation plan, specification, commitment to pursue
Common Criteria, or final architectural proposal. It does not authorize a
claim that Xenomorph is Common Criteria conformant, evaluated, certified, or
associated with any Evaluation Assurance Level (EAL).

The conclusion in this document is provisional. It applies to the repository
state examined on 2026-07-14 and may need revision when the intended operating
environment, operator model, or client tamper-resistance objective is defined.

## Scope and Evidence Boundary

This assessment is based on the source tree, tracked configuration material,
tests, and CI configuration. It does not establish the security of a deployed
environment, the correctness of every runtime path, the effectiveness of
operational controls, or the acceptability of any assurance claim to a
national certification scheme.

## Major Assumptions

The following assumptions remain unsupported by repository evidence and must
not be treated as facts.

- **Assumption:** The Xenomorph team, rather than an external customer, is the
  intended relying party for any assurance work. The stated intent is to use a
  government-like assurance posture internally.
- **Assumption:** The intended deployment may eventually require controls
  resistant to local client tampering. The repository contains no agreed
  attacker model, tamper-resistance objective, hardware assumptions, or
  supported operating-system configuration for that goal.
- **Assumption:** A Common Criteria scheme, protection profile, or EAL would
  be relevant to the team’s own decision-making. No applicable scheme,
  procurement requirement, protection profile, or evaluator has been
  identified.
- **Assumption:** Future client tamper protection would be in scope for the
  same assurance target as the gateway. That decision is not yet made and may
  be inappropriate because the client and gateway have different trust roles.

## Common Criteria Context

Common Criteria is the evaluation framework described by ISO/IEC 15408. EAL1
through EAL7 are assurance packages with increasing evaluation rigor. An EAL
does not independently state that a product is secure for every deployment;
the evaluated target, security claims, configuration, and operating
environment determine what an evaluation means. The applicable national scheme
and protection profile can further constrain the claim.

For Xenomorph, the relevant question is not whether “the project” has an EAL.
It is whether a stable, bounded target and a precisely stated internal
assurance decision exist. Neither is presently established.

## Repository Findings

### Component ownership and trust assertions

The gateway owns agent authentication, identity derivation, command signing,
event normalization, and broker publication. It requires TLS 1.3 and a
verified client certificate on every agent-facing HTTP route. The gateway
derives the agent identifier from the presented certificate fingerprint, not
from a client payload. It writes gateway-authored event identifiers,
timestamps, session identifiers, and authentication context into event
envelopes.

The agent owns local telemetry collection and local execution of accepted
commands. Heartbeat data, endpoint attestations, logs, screenshots, screen frames,
terminal output, filesystem observations, and filesystem operation results are
client-authored. A valid client certificate authenticates the certificate to
the gateway; it does not make these observations trustworthy evidence of host
integrity.

The browser dashboard owns presentation and the submission of operator-origin
requests. In the current implementation, the gateway labels dashboard-initiated
operations with the configured `FILE_OPERATOR_ID` value or the fixed
`"website"` label. These values are not authenticated human identities.

NATS JetStream owns durable storage of published event messages. The broker is
an external dependency from the gateway’s perspective. The current source
creates the NATS connection from `NATS_URL` without configuring TLS or client
certificates.

### Connection classification

| Connection | Present protocol and authentication | Data and trust classification | Assessment |
| --- | --- | --- | --- |
| Agent to gateway ingestion and command plane | HTTPS with TLS 1.3, server-name validation by the client, and gateway-side `RequireAndVerifyClientCert`. | The certificate-derived agent ID and event envelope metadata are gateway-authored. Heartbeats, endpoint attestations, logs, command results, and transfer bytes are client-authored. | This is the intended agent trust boundary. It authenticates a certificate, not the integrity of the executing client. |
| Agent to gateway screen-media plane | WebSocket over the agent’s configured TLS client settings; the same gateway mTLS middleware applies before upgrade. | Frame bytes and declared content type are client-authored opaque media. The gateway stores them in memory. | Transport identity is bound to mTLS; media truthfulness and client integrity are not established. |
| Agent to gateway transfer data plane | mTLS HTTP routes plus a short-lived bearer capability bound to an agent and transfer. | Transfer manifest and lease are gateway-authored; uploaded or downloaded file bytes and client-side transfer results are client-authored. | The source checks agent scope, lease, chunk bounds, and hashes. It does not establish that client-originated bytes describe an untampered host. |
| Browser dashboard to gateway dashboard API | HTTPS with TLS 1.3 server configuration. No application authentication or authorization middleware is registered. | Browser requests are operator-authored input; the runtime records a shared configured operator label rather than an authenticated operator identity. | This is an administrative control path that can queue terminal commands, screen requests, filesystem operations, and transfers. TLS and WebSocket origin checking do not authenticate an operator. |
| Browser dashboard live-screen WebSocket | WebSocket from the dashboard listener; origin must equal the configured dashboard origin. | Screen frames remain client-authored; requests to view them are unauthenticated browser actions. | Origin checking mitigates some cross-origin browser use but is not user authentication or authorization. |
| Gateway to NATS JetStream | The default is `nats://localhost:4222`; the broker constructor uses `nats.Connect(url)` without TLS options. | Event envelopes are gateway-authored wrappers containing client-authored payloads. | The repository does not demonstrate protected service-to-service transport or broker client authentication. This contradicts the repository security standard’s stated NATS TLS expectation. |
| Local gateway state | Filesystem-backed operation and transfer records under `GATEWAY_STATE_PATH`; command queues and several dashboard stores are in memory. | Gateway-created operation state is server-authored; stored transferred content can include client- or operator-authored bytes. | Persistence, access control, encryption at rest, backup, and retention characteristics are deployment-dependent and not established here. |
| Local client state and credentials | Client runtime state is stored under the user home directory. The client loads certificate material and the command verification certificate from a local certificate path. | Replay records are local client state; local credential and trust-anchor files determine the client’s ability to authenticate and verify commands. | No examined mechanism protects these local files from a sufficiently privileged local adversary. |

### Present command integrity controls

The gateway signs command envelopes with RSA-PSS using its server private key.
The command contains a protocol version, command ID, certificate-derived
audience ID, type, payload, requesting label, issue and expiry timestamps,
nonce, reason, key ID, and signature. The agent verifies the signature against
the public key in its local server certificate, enforces the audience, expiry
window, allowed command type, key ID, and a bounded persisted nonce history.

These controls provide command-origin and replay protections only while the
agent’s executable, local verification material, process, and persistent state
remain trustworthy enough to enforce them. They do not solve client tampering.
A local adversary able to modify the client binary, its loaded certificate
material, or its runtime state may be able to bypass the local decision logic
or falsify client-authored results. The repository does not currently contain
binary signing verification, measured boot, hardware-backed key binding,
remote attestation, anti-debugging, integrity monitoring, or a defined
response to detected tampering. This is a finding about the examined source,
not a claim that no compensating control exists in deployment.

### Current assurance-relevant gaps

The following are present-state observations, not implementation instructions.

- The dashboard listener is an administrative API with state-changing routes,
  but its handlers do not authenticate an operator or apply per-operator
  authorization. A loopback default address reduces exposure in the default
  configuration; it does not create an authenticated administrative boundary.
- The NATS broker connection has no TLS or client-certificate configuration in
  the examined source. The supplied development compose file exposes NATS on
  its default port without shown authentication or TLS settings.
- Certificate authority, server, and client private-key material are tracked
  under `platform/infrastructure/certs`. The certificate-generation script
  also creates unencrypted development keys. This is incompatible with a claim
  that repository-tracked credentials are protected production secrets.
- The client and gateway defaults are local-development oriented, including
  `localhost` endpoints and relative certificate paths. This is not a bounded,
  reproducible evaluated configuration.
- CI runs Go formatting, tests, race tests, vet, static analysis,
  vulnerability checks, gosec, and cross-platform Go builds. The examined CI
  workflow does not build or test the website, and the repository does not
  show a release-signing, artifact-attestation, or software-update trust
  chain.
- The command queue and several dashboard stores are in memory; queued
  commands are intentionally lost on gateway restart. File-operation and
  transfer records are filesystem-backed. The durability and recovery model is
  therefore mixed and must be bounded before it could support a formal target.

## Problems Separated from Solutions

### Problems

The first problem is not a missing EAL. It is that the platform’s most
security-critical authority—the dashboard operator—is not represented by a
verified identity in the present runtime. Any assurance argument for terminal
execution, screen access, or filesystem mutation would be incomplete without
an operator authentication and authorization boundary.

The second problem is that the client is intended to acquire substantial
tamper protection later, but that future security objective is unspecified.
Client tamper resistance depends on the attacker’s local privilege, physical
access, operating system, supported hardware, enrollment process, recovery
model, and the consequences of failed integrity evidence. Without those
constraints, an EAL selection or client assurance claim would be arbitrary.

The third problem is target instability. The client currently includes command
execution, screen capture and streaming, filesystem inspection and mutation,
and transfer features; its role is materially broader than telemetry
collection. The gateway, browser dashboard, NATS deployment, local state, and
certificate lifecycle each influence the security outcome. A formal evaluation
would need an intentionally narrow target and fixed configuration, neither of
which exists.

The fourth problem is evidence maturity. The repository has useful source
comments, validation code, tests, and a Go-focused CI pipeline, but those are
not an assurance case. They do not yet show controlled production secrets,
operator accountability, protected service-to-service transport, deployment
hardening, vulnerability response, or lifecycle management.

### Possible Directions

| Direction | What it would answer | Limitation |
| --- | --- | --- |
| Internal assurance baseline | Whether the team can make evidence-backed internal decisions about the gateway, dashboard, broker, client, and operations. | It does not create a formal Common Criteria claim. |
| Gateway-focused evaluation exploration | Whether the fixed gateway ingress and command-authoring boundary could become a bounded assurance target. | It would not establish the integrity of the client or the safety of unauthenticated dashboard actions. |
| Client tamper-resistance research | Which client threats can realistically be resisted or detected, and what evidence the gateway could rely on. | It cannot be scoped responsibly without a specific local attacker model and supported platform assumptions. |
| Formal Common Criteria evaluation | Whether an identified scheme and defined target justify externally recognized evaluation rigor. | It is premature while the target, deployment configuration, operational controls, and client integrity objective are unsettled. |
| Disregard all formal assurance work | Avoids certification overhead. | It would discard useful internal discipline for a system that performs remote terminal, screen, and filesystem operations. |

## Provisional Decision

Formal Common Criteria evaluation, including selecting an EAL, should be
disregarded for the current repository state. This is a decision to defer a
formal certification path, not a decision to disregard assurance.

The reason is not that the team lacks an external customer. An internal team
with government-like assurance needs can be a legitimate relying party.
Instead, no stable target of evaluation or internal decision criterion has
been defined, and present trust-boundary gaps would make an EAL label less
informative than an evidence-backed account of actual controls. Selecting an
EAL now could encourage a misleading focus on a certification label while the
dashboard authority boundary, broker transport, credential handling, and
client tamper-resistance objective remain unresolved.

The appropriate near-term posture is to retain this exploration and use
measurable internal assurance evidence. In particular, the gateway must
continue to treat all client-originated telemetry, media, filesystem evidence,
and command results as untrusted regardless of future client tamper defenses.
Tamper resistance may increase confidence in specific client claims; it cannot
move the gateway’s authentication, authorization, or identity authority to the
client.

## Information Required Before a Formal Proposal or Plan

This exploration could become a formal proposal or plan only when the
following information is available:

- the internal assurance decision the team needs to make and the people
  authorized to rely on it;
- a written attacker model for client tampering, including local privilege,
  physical access, supported operating systems and hardware, expected
  detection or resistance properties, and the response to failed evidence;
- a proposed target of evaluation with fixed component versions, interfaces,
  deployment configuration, external dependencies, and explicit exclusions;
- a defined authenticated operator identity, authorization model, and audit
  accountability boundary for every dashboard-initiated action;
- verified deployment evidence for certificate lifecycle, secret handling,
  gateway-to-broker protection, state storage, logging, monitoring, recovery,
  and vulnerability response;
- traceable evidence that implemented controls behave as claimed, including
  their failure modes and residual risks; and
- an identified Common Criteria scheme, protection profile or other assurance
  approach that the team has decided is suitable for that defined target.
