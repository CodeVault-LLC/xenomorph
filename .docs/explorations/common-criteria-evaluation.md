--
title: Common Criteria Evaluation
status: exploration
created: 2026-07-14
last_updated: 2026-07-14

---

# Common Criteria Evaluation

## Exploratory and Non-Binding Status

This document preserves an idea for investigation. It is not an approved
feature, implementation plan, specification, commitment to pursue Common
Criteria, or final architectural proposal. No Evaluation Assurance Level
(EAL), evaluation scope, sponsor, laboratory, certification target, or
security target has been selected for Xenomorph.

The questions below are intended to determine whether any evaluation-oriented
work would provide meaningful assurance for this internal remote screening
platform. They do not authorize a claim of Common Criteria conformance or
certification.

## Major Assumptions

The following assumptions are not yet supported by evidence in this
repository. They are recorded explicitly because they materially affect the
value and feasibility of any future direction.

- **Assumption:** A government, defense, regulated-sector, or similarly
  assurance-sensitive customer may require independent evidence about
  Xenomorph security controls. No such requirement is currently documented.
- **Assumption:** An EAL-based Common Criteria evaluation could be relevant
  to the platform. Applicability depends on the prospective deployment,
  procurement rules, target market, and national certification scheme.
- **Assumption:** The current deployment boundary, operational environment,
  and supported configurations can be defined precisely enough for an
  evaluation target. The repository documentation describes a gateway, agent,
  and shared schema, but does not establish a certification scope.
- **Assumption:** Existing documentation and tests may provide some evidence
  useful to an assurance effort. Their completeness, independence, and
  acceptability to an evaluator are unknown.

## Context

Common Criteria is the evaluation framework described by ISO/IEC 15408. EAL1
through EAL7 are assurance packages associated with increasing evaluation
rigor; an EAL is not, by itself, a statement that a product is secure for a
particular deployment. An evaluation is normally scoped to a defined target
of evaluation, its security claims, and its operating environment. The exact
meaning and availability of a given evaluation can vary with the applicable
national scheme and protection profile.

Xenomorph currently documents a trust boundary in which the gateway is the
sole authority for authentication, identity derivation, authorization, event
provenance, and command dispatch. The client is an untrusted emitter of
client-authored telemetry. Agent-to-gateway transport is documented as mTLS,
and the gateway derives identity from certificate material before constructing
a server-authored event envelope. These are documented system properties, not
evidence that any Common Criteria requirement is satisfied.

## Problems to Understand

### Assurance needs are not yet established

It is not known whether prospective users, operators, or procurement channels
need Common Criteria evaluation, another formal assurance mechanism, or only
clearer internal security evidence. Pursuing a formal evaluation without a
defined relying party could consume attention while providing little practical
assurance.

### Security claims need a bounded subject

The platform spans an agent, a gateway, a broker integration, shared event
schema, certificates, operational configuration, and downstream consumers.
An assurance claim cannot be evaluated meaningfully until it is clear which
components, versions, configurations, deployment assumptions, and external
dependencies would be in scope. In particular, the gateway’s trust-boundary
role may make it more relevant than client telemetry collection, but that is
an untested scoping hypothesis.

### Current evidence quality is unknown

The repository contains security rules and architecture documentation,
including strict certificate validation, server-authored envelopes, input
validation, audit logging, and secure development expectations. It is unknown
whether the implemented behavior, test coverage, build provenance,
configuration control, vulnerability handling, and operational evidence would
support any independent evaluation. Documentation of a control must not be
treated as proof that the control is implemented or effective.

### Formal terminology can create misleading expectations

Using terms such as “EAL,” “evaluated,” “certified,” or “Common Criteria
compliant” without a completed evaluation could overstate the platform’s
assurance. This is especially consequential because client-authored telemetry
must not be represented as trusted input and because the gateway is the sole
trust boundary.

### Assurance effort may not address the most important risks

An EAL-oriented effort might emphasize artifacts and evaluation scope while
leaving practical risks insufficiently measured, such as certificate lifecycle
operations, deployment hardening, broker security, operator access, incident
response, or the treatment of sensitive telemetry. The relative importance of
these risks is not yet known.

## Possible Directions

None of the directions below has been selected. They may be complementary,
but their cost, value, and suitability cannot yet be compared quantitatively.

| Direction | Potential value | Open questions and constraints |
| --- | --- | --- |
| Remain with internal security assurance | Could focus on demonstrated gateway controls, threat modeling, tests, and operational evidence without implying formal certification. | Would this satisfy the actual assurance needs of intended users or procurement channels? What independent review, if any, would be credible? |
| Map existing controls to Common Criteria concepts | Could clarify terminology, likely scope boundaries, evidence gaps, and whether a Common Criteria path is plausible. | A conceptual mapping is not an evaluation and must not be presented as conformance. Which scheme, protection profile, or target would make the mapping relevant? |
| Explore a narrowly bounded evaluation target | Could test whether an evaluation focused on a defined gateway configuration provides useful assurance for the trust boundary. | Would excluding the agent, broker, dashboard, or operations create an assurance claim too narrow for the intended use? Is a suitable evaluator and scheme available? |
| Consider a formal Common Criteria evaluation | Could provide externally recognized assurance when a specific market or procurement requirement justifies it. | What EAL or protection profile, scope, evidence, maintenance obligations, schedule, cost, and organizational ownership would apply? No level should be presumed. |
| Consider other assurance mechanisms | A different standard, independent assessment, customer review, or secure-development attestation may better address the actual risk and market need. | Which mechanism is accepted by relevant relying parties, and how would it cover technical controls and operational practices? |

## Research Questions and Potential Evidence

The immediate work, if this exploration continues, should be research and
measurement rather than implementation.

- Which named users, customers, regulators, or procurement frameworks require
  Common Criteria evaluation, specify a protection profile, or accept an
  alternative assurance mechanism?
- What security decision would a future evaluation enable, and who would rely
  on it?
- What target of evaluation could be stated without misrepresenting the
  platform? This should distinguish gateway-authored trust data from
  client-authored payload data and identify external dependencies.
- Which documented controls are demonstrably implemented, tested, and
  traceable to source, build, configuration, and operational evidence?
- Which deployment assumptions are security-relevant, including certificate
  issuance and revocation, gateway administration, NATS JetStream security,
  network reachability, and operator authentication?
- Is there an applicable Common Criteria scheme or protection profile for the
  intended deployment, and what assurance activities would it require?
- How would an evaluated configuration be versioned, reproduced, and kept
  distinct from configurations outside the evaluated scope?
- What residual risks would remain outside a possible evaluation scope?

Small, non-production prototypes or evidence reviews may be useful only if
they answer one of these questions. For example, a bounded evidence inventory
could measure traceability from a gateway trust-boundary claim to existing
tests and documentation; it would not establish certification. Any prototype
must preserve the gateway as the sole ingestion trust boundary and must not
weaken certificate validation.

## Information Required Before a Formal Proposal or Plan

This exploration could become a formal proposal or plan only after the
following information is available:

- a documented business, customer, regulatory, or procurement need;
- identified relying parties and the assurance decision they need to make;
- a proposed and reviewable target of evaluation, including components,
  versions, configuration, deployment environment, and dependencies;
- the applicable Common Criteria scheme, protection profile or assurance
  approach, and any constraints on the intended claim;
- evidence from a gap assessment of implemented controls, tests,
  documentation, build and release integrity, configuration management, and
  operational processes;
- an explicit statement of out-of-scope components, assumptions, residual
  risks, and ownership boundaries; and
- authorization to make any external assurance claim and to commit the
  required organizational resources.
