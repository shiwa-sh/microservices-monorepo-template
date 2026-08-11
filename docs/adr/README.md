# Architecture Decision Records

Every decision that binds more than one service, or is hard to reverse, is recorded here. ADR-0000 carries the thesis, the principles, and the process; [ADR-0001](0001-documentation-and-output-conventions.md) carries the rules these documents are written to.

Writing a new one starts from [`_template.md`](_template.md).

Every ADR carries a **Decides** line in its header: one declarative sentence naming what is true of the platform because of it. The *Decides* column below is that line, shortened to the index. Reading the column is the fastest complete pass over the set. `(CI: lint:adr-xref)`

## Numbering

Numbers are allocated in **blocks of a hundred, one block per layer**, sequential within the block. The first two digits carry the layer, so a new ADR lands in its block without renumbering the set. Gaps are deliberate.

| Block | Layer |
| --- | --- |
| `00xx` | Foundations and conventions |
| `01xx` | Repository and delivery |
| `02xx` | Infrastructure |
| `03xx` | Application platform |
| `04xx` | Interfaces |
| `05xx` | Observability |
| `06xx` | Development loop |
| `07xx` | Product |

A hundred slots per layer rather than ten, so a layer can grow without renumbering the set. The index below is the set; the blocks are not counted here, so adding an ADR touches one table.

## The set

### 00xx — Foundations and conventions

| ADR | Title | Decides |
| --- | --- | --- |
| [0000](0000-platform-foundations.md) | Platform Foundations | The three axes and this platform's position on them, the principles, and the ADR process |
| [0001](0001-documentation-and-output-conventions.md) | Documentation & Output Conventions | How docs, ADRs, logs, CLI output, and code comments are written |
| [0002](0002-tool-adoption.md) | Tool Adoption & Comparison Requirement | The three exit-cost tiers, the depth of comparison each owes, and the tool register |
| [0003](0003-naming-and-identifiers.md) | Naming & Identifiers | The resource slug grammar, entity identifiers, and casing per surface |

### 01xx — Repository and delivery

| ADR | Title | Decides |
| --- | --- | --- |
| [0100](0100-language-and-runtime.md) | Language & Runtime | Go and TypeScript, and what may not be added |
| [0101](0101-monorepo.md) | Monorepo Structure & Build | Layout, task runner, affected detection, caching |
| [0102](0102-source-control-and-ci.md) | Source Control & CI Platform | The forge, where pipelines run, and the build identity |
| [0103](0103-release-and-versioning.md) | Release, Tagging & Versioning | Conventional Commits, CalVer, image tags |
| [0104](0104-supply-chain-security.md) | Supply-Chain Security | Signing, scanning, admission verification |
| [0105](0105-image-registry.md) | Image Registry | Where images and their attestations live |
| [0106](0106-dependency-updates.md) | Dependency Updates & Template Propagation | What moves the pins, and how a template fix reaches a generated project |

### 02xx — Infrastructure

| ADR | Title | Decides |
| --- | --- | --- |
| [0200](0200-cluster-topology.md) | Cluster Topology & Hosting | Where production runs, what a node is, and what recovering a cluster costs |
| [0201](0201-gitops.md) | GitOps & Deploy | Argo CD as the delivery engine, repository topology, sync policy |
| [0202](0202-secrets.md) | Secrets Management | SOPS and age, the recipient model, key lifecycle |
| [0203](0203-policy-enforcement.md) | Policy Enforcement Strategy | Which layer enforces which class of invariant, and how each layer fails |
| [0204](0204-resource-management.md) | Resource Management & Scheduling | Requests, limits, priority classes, quotas |
| [0205](0205-environment-parity.md) | Environment Parity | What may differ across environments, what may not, and the PR preview tier |
| [0206](0206-cluster-networking.md) | Cluster Networking | The CNI, the day-one postures, and why no service mesh |
| [0207](0207-cluster-storage.md) | Cluster Storage & Backups | Node-local volumes, one object store everywhere, and where backups live |

### 03xx — Application platform

| ADR | Title | Decides |
| --- | --- | --- |
| [0300](0300-data.md) | Data & Migrations | Postgres, CNPG, sqlc, migrations, multi-tenancy |
| [0301](0301-data-lifecycle-privacy.md) | Data Lifecycle & Privacy | Retention, erasure, subject access |
| [0302](0302-temporal.md) | Durable Execution (Temporal) | What earns a workflow, and how workers deploy |
| [0303](0303-api-contracts-and-lifecycle.md) | API Contracts, Codegen & Lifecycle | OpenAPI 3.1, ogen, audience, versioning |
| [0304](0304-identity-and-authorization.md) | Identity & Authorization | Kratos, Hydra, OpenFGA, organisations |
| [0305](0305-edge-auth-and-traffic-policy.md) | Edge Authentication & Traffic Policy | Oathkeeper forward-auth, rate limits, security headers |
| [0306](0306-trust-tiers-and-urls.md) | Trust Tiers & URL Structure | The `ops.` boundary, cookies, routing, the flat `/api` path |
| [0307](0307-outbound-email.md) | Outbound Email | The sending path identity flows depend on, and its pre-built exit |

### 04xx — Interfaces

| ADR | Title | Decides |
| --- | --- | --- |
| [0400](0400-frontend.md) | Frontend Stack & Conventions | Next.js, rendering, styling, forms, CSP, portals |
| [0401](0401-internal-admin.md) | Internal Admin Tool | Lowdefy over the API, and the write-path invariant |

### 05xx — Observability

| ADR | Title | Decides |
| --- | --- | --- |
| [0500](0500-observability.md) | Observability | OpenTelemetry, the Grafana backend, cardinality discipline |
| [0501](0501-operator-uis-and-dashboards.md) | Operator UIs & Dashboard Hierarchy | Which UI answers which question, and the L1–L3 funnel |
| [0502](0502-alerting-and-on-call.md) | Alerting & On-Call | Where alerts route, and where escalation is conceded |
| [0503](0503-error-tracking.md) | Error Tracking | Errors as OTel data, fingerprint grouping, and why no tracker joins the floor |

### 06xx — Development loop

| ADR | Title | Decides |
| --- | --- | --- |
| [0600](0600-local-development-loop.md) | Local Development Loop | The two local tiers, dependency graph, service contract, API mock |
| [0601](0601-testing-strategy.md) | Testing Strategy | Correctness layers, the acceptance gauge, load testing |

### 07xx — Product

| ADR | Title | Decides |
| --- | --- | --- |
| [0700](0700-analytics.md) | Marketing & Product Analytics | One browser agent, the Collector split, the analytics store |

## Registries

Three documents are named by an ADR as the only place a fact is recorded, so a decision links to them rather than restating what they hold.

| Document | Holds |
| --- | --- |
| [`operational-surface.md`](../operational-surface.md) | every platform component, its tier, and what it obliges |
| [`adoption-path.md`](../adoption-path.md) | the ranked order in which the floor is reduced |
| [`tool-register.md`](../tool-register.md) | every tool, its exit-cost tier, licence, governing body, and owning ADR |
