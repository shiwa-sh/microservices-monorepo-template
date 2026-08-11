# Per-Instance Hardening & Compliance

[`security-baseline.md`](../security-baseline.md) is the floor every project inherits, generated from the Rules sections that own each control. This document is the other half: what a **particular** project turns on for its own risk profile or its own compliance framework.

It is written rather than generated because its subject is the instance, not the template. Nothing here is a gap in the baseline — each row is a control the template declines to impose on every project, with the condition that makes it necessary.

## Conscious omissions from the baseline

Each is a values or policy change a project makes when the condition holds.

| Omitted | Why it is not a default | Turn it on when |
| --- | --- | --- |
| **Per-account lockout and CAPTCHA** | The edge IP rate limit covers the untargeted case, and per-account backoff punishes the account being attacked as readily as the attacker ([ADR-0304](../adr/0304-identity-and-authorization.md)) | Credential stuffing targets named accounts, or the product's user base makes an account takeover materially worse than a lockout |
| **The HaveIBeenPwned breach check** | Its client ignores network errors by default, so on a default-deny cluster it fails open silently and the registration pays a latency tax for a check that did not run ([ADR-0304](../adr/0304-identity-and-authorization.md)) | The egress allowance for it exists and is verified. A control that reads as enabled and is inert on the wire is worse than one honestly off |
| **Ops-tier token isolation** | The parent-scoped session cookie is accepted while every origin under the host is first-party and edge-gated ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)) | The product surface renders content one user supplies to another, or anything not first-party is hosted under the apex. [`adoption-path.md`](../adoption-path.md) ranks the two forms |
| **`__Secure-` cookie prefix** | Renaming the session cookie touches Kratos, the Oathkeeper rule set, and the frontend proxy together, and the prefix adds no isolation the `Secure` attribute lacks | The rename is being made for another reason. `__Host-` stays inapplicable: it forbids the `Domain` attribute the tier model requires |
| **B2C MFA, social login, SCIM** | Each is a product decision rather than a platform one ([ADR-0304](../adr/0304-identity-and-authorization.md)) | The product requires it. Operator AAL2 is baseline either way |
| **Per-workload certificate identity** | Positional trust is the recorded deviation from NIST SP 800-207, bounded by default-deny and the `restricted` profile ([ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md)) | A service performs a monetary mutation, a second team owns a service, or an auditor requires a CA chain |

## Which scanner an instance runs

[ADR-0104](../adr/0104-supply-chain-security.md) fixes that vulnerability scanning is a merge gate and leaves the scanner to the instance. The choice is recorded here, with its ignore policy and its failure threshold, because those are the parts a reviewer needs and neither is a property of the template.

## Reaching a named compliance framework

A framework is reached by layering on top of the baseline, without forking the template. Each row is a values overlay, a policy change, or a CI addition.

| Requirement | Where it lands |
| --- | --- |
| Audit retention over the framework's window | the per-environment observability values. The authorization decision events already flow to the log store ([ADR-0500](../adr/0500-observability.md)) |
| Network segmentation beyond default-deny | tighter CiliumNetworkPolicy per namespace for the regulated data path ([ADR-0200](../adr/0200-cluster-topology.md)) |
| Encryption at rest | the storage class and the database values in the per-environment infrastructure. The template is provider-agnostic and states no default |
| Stronger authentication | required MFA, shorter session lifetimes, and the ops-tier token isolation above |
| Periodic access review | a scheduled review of the operator group and dashboard relations, which OpenFGA holds as the source of truth ([ADR-0304](../adr/0304-identity-and-authorization.md)) |
| Vulnerability management with demonstrable continuity | a scanner's findings are a merge gate today; continuous fleet-wide triage against images already built is a component this platform does not run ([`operational-surface.md`](../operational-surface.md)) |
| Penetration-test attestation | a CI artefact and a schedule. Nothing in the template changes |

**PCI DSS is scope-triggered rather than layered.** The platform stores no cardholder data and a payment integration hands off to a provider. A project that takes card data directly brings the scope with it, and that changes [ADR-0300](../adr/0300-data.md) and [ADR-0301](../adr/0301-data-lifecycle-privacy.md) rather than this document ([ADR-0000](../adr/0000-platform-foundations.md)).

## The verification obligation

A control claimed and never examined ages into a control believed. [ADR-0304](../adr/0304-identity-and-authorization.md) sets the ASVS bar and the cadence; [`asvs-verification.md`](asvs-verification.md) holds the per-row verdicts and the date each was last examined. A row in this document that a project turns on joins that table, because a hardening nobody verifies is the same claim as a baseline control nobody verifies.
