# Threat Model

The controls are decided and enforced; what no document states is **who they are against**. A control set without a named adversary is reviewable only by the person who designed it, because everyone else is guessing at the assumption each control was written under.

This is STRIDE-lite over the three boundaries that matter here. It decides nothing: every control links to its ADR, and every accepted gap links to [`risk-register.md`](risk-register.md).

## Adversaries

| Adversary | Capability assumed | What they want |
| --- | --- | --- |
| **Unauthenticated internet** | can reach every published origin, at any rate, forever | credentials, data, a foothold, or the cost of the platform's capacity |
| **Authenticated user** | a valid session in one organisation | another organisation's data, or authority they were not granted |
| **Compromised first-party pod** | code execution inside a workload the platform runs, with that pod's network allowances | lateral movement to the data tier, cloud credentials, persistence |
| **Compromised build path** | can land a commit, or influence a dependency | a malicious image running in production |
| **Stolen operator session** | a browser holding an operator's session | the ops tier, which is the cluster's control surface |
| **Provider or physical access** | the infrastructure underneath | data at rest, and the machines |

**Not modelled.** A nation-state adversary with supply-chain and hardware capability, an insider with the SOPS recovery key, and a compromised upstream image publisher whose signature verifies. Each would defeat controls here, and each is excluded deliberately rather than by oversight ([ADR-0000](../adr/0000-platform-foundations.md) makes the same exclusion when it declines TUF).

## Boundary 1 — the internet, and the edge

| Threat | Control | Owner |
| --- | --- | --- |
| **Spoofing** identity by supplying identity headers | Oathkeeper strips every client-supplied identity header before injecting authoritative ones | [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md) |
| **Spoofing** a session by forging or replaying a token | tokens validated once at the edge with the algorithm pinned and `iss`, `aud`, `exp` checked; sessions are `Secure`, `HttpOnly`, `SameSite=Lax` | [ADR-0304](../adr/0304-identity-and-authorization.md) |
| **Tampering** with a request in transit | TLS at the edge, verified in every environment | [ADR-0205](../adr/0205-environment-parity.md) |
| **Repudiation** of an authorization decision | per-decision events from the authorization service into the log store | [ADR-0304](../adr/0304-identity-and-authorization.md) |
| **Denial of service** by brute force or volume | rate limiting at the edge, per route, with a tighter limit on the authentication paths | [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md) |
| **Elevation** through a cross-origin or cross-site request | strict per-request nonce CSP, `frame-ancestors 'none'`, an origin check on server actions, and Kratos's own anti-CSRF token on self-service flows | [ADR-0400](../adr/0400-frontend.md), [ADR-0304](../adr/0304-identity-and-authorization.md) |
| **Elevation** from a product session into the ops tier | the ops coarse gate requires an `operator` claim and AAL2 — a product session alone reaches nothing | [ADR-0306](../adr/0306-trust-tiers-and-urls.md) |
| **Information disclosure** through a stolen operator session riding an XSS | the product-origin CSP, and nothing else. **Accepted risk 1** | [risk 1](risk-register.md) |

## Boundary 2 — inside the cluster

| Threat | Control | Owner |
| --- | --- | --- |
| **Spoofing** identity between services | none cryptographic. A service trusts `X-User-Id` because default-deny governs who can reach its port. **Accepted risk 2** | [risk 2](risk-register.md) |
| **Elevation** by a compromised pod reaching the data tier | default-deny east-west, so reachability is an allowance rather than a default | [ADR-0200](../adr/0200-cluster-topology.md) |
| **Information disclosure** by capturing east-west traffic | WireGuard encryption of all pod traffic | [ADR-0200](../adr/0200-cluster-topology.md) |
| **Elevation** to the node from a pod | Pod Security Admission `restricted` in every namespace, with a pinned enforce-version, on an immutable node with no shell and no package manager | [ADR-0200](../adr/0200-cluster-topology.md) |
| **Information disclosure** of cloud credentials through the metadata endpoint | egress policy denies the metadata address specifically | [ADR-0200](../adr/0200-cluster-topology.md) |
| **Elevation** across tenants in one database | one logical database per service, per-service credentials, and no service reaching another's data | [ADR-0300](../adr/0300-data.md) |
| **Elevation** by an authenticated user reaching another organisation's resources | every read and every mutation authorized through `Checker` as though the identifier were public | [ADR-0304](../adr/0304-identity-and-authorization.md), [ADR-0003](../adr/0003-naming-and-identifiers.md) |
| **Tampering** with the authorization store through a side channel | authz-relevant writes are dual-written inside a workflow, never by an ad-hoc path | [ADR-0302](../adr/0302-temporal.md), [ADR-0401](../adr/0401-internal-admin.md) |

## Boundary 3 — the build and deploy path

| Threat | Control | Owner |
| --- | --- | --- |
| **Tampering** with an image between build and run | cosign signature and required attestations verified at admission; every reference digest-pinned | [ADR-0104](../adr/0104-supply-chain-security.md), [ADR-0203](../adr/0203-policy-enforcement.md) |
| **Spoofing** the origin of an image | signing key held in SOPS, never by a person; the public key is committed and named by the admission policy | [ADR-0104](../adr/0104-supply-chain-security.md) |
| **Tampering** with cluster state outside the repository | Argo CD is the only deploy mechanism, and production does not self-heal, so drift is surfaced rather than silently corrected | [ADR-0201](../adr/0201-gitops.md) |
| **Elevation** through a dependency with a known vulnerability | scanning as a merge gate, with the finding blocking before anything ships | [ADR-0104](../adr/0104-supply-chain-security.md) |
| **Information disclosure** of secrets in the repository | SOPS with age recipients; plaintext never committed, and the local key decrypts only throwaway values | [ADR-0202](../adr/0202-secrets.md) |
| **Denial of service** by the gate itself | Kyverno's webhook fails closed cluster-wide. **Accepted risk 5**, with a documented break-glass | [risk 5](risk-register.md) |

## What the model shows

**The strongest boundary is the outermost one, and the weakest is the middle.** The edge validates, strips, and injects; inside the cluster, identity is a header believed because of where it came from. That asymmetry is the deliberate trade in [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md), and it is why *compromised first-party pod* is the adversary to read first: it is the one whose capability the design most depends on being rare.

**Every gap in these tables is a row in the register, and every register row has a trigger.** A threat model that ends in a list of unmanaged worries is a document nobody reads twice.
