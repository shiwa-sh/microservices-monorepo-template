# Tool Register

Every tool this platform runs, builds with, or generates from. [ADR-0002](adr/0002-tool-adoption.md) makes this the canonical inventory: a tool's tier, licence, governing body, and owning ADR are recorded here and nowhere else.

**This is not a decision surface.** A row states facts and points at the ADR that chose the tool. The reasoning, the alternatives, and the verdict live there.

**Tier is exit cost** ([ADR-0002](adr/0002-tool-adoption.md)): **1** structural, months and possibly customer data; **2** substitutable, weeks behind a stable interface; **3** library, days inside one package. The tier sets what comparison the owning ADR owes, and *Recorded against* names the alternatives that comparison holds.

**Licence and governance are recorded and do not veto** ([ADR-0000](adr/0000-platform-foundations.md), principle 4). They are evidence about exit cost: a single-vendor project under a source-available licence has a different abandonment profile from a foundation-governed one, and that difference is what the column is for. Both are third-party facts, so they are re-verified on the cadence in [`reference/upstream-status.md`](reference/upstream-status.md), and a change to either is recorded there rather than edited quietly here.

## Tier 1 — structural

Leaving is a rewrite or a multi-month migration. The owning ADR carries a full comparison table and names a runner-up.

| Tool | Concern | Owning ADR | Licence | Governance | Runner-up | Recorded against |
| --- | --- | --- | --- | --- | --- | --- |
| Go | backend language | [0100](adr/0100-language-and-runtime.md) | BSD-3-Clause | Google | Kotlin on the JVM | Rust, C#/.NET, Kotlin, Java, TypeScript on the backend |
| TypeScript | frontend language | [0100](adr/0100-language-and-runtime.md) | Apache-2.0 | Microsoft | none — the browser sets it | JavaScript, ReScript, Elm |
| Bun | frontend runtime and package manager | [0100](adr/0100-language-and-runtime.md) | MIT | Oven, single-vendor | Node.js | Node.js, Deno, pnpm on Node |
| Next.js | frontend framework | [0400](adr/0400-frontend.md) | MIT | Vercel, single-vendor | TanStack Start | TanStack Start, React Router v7, Astro, SvelteKit, Nuxt |
| Untitled UI React + Tailwind | design system | [0400](adr/0400-frontend.md) | MIT for the React library; the Figma kit and PRO tiers are commercial | Untitled UI, single-vendor | shadcn/ui | shadcn/ui, Mantine, Park UI, Material UI, headless-only |
| PostgreSQL | relational store | [0300](adr/0300-data.md) | PostgreSQL Licence | PostgreSQL Global Development Group | none — no option survived the constraints | MySQL, MariaDB, CockroachDB, YugabyteDB, SQLite |
| CloudNativePG | Postgres operator | [0300](adr/0300-data.md) | Apache-2.0 | CNCF | StackGres | Zalando, Percona, StackGres, Crunchy, a hand-rolled StatefulSet |
| Kubernetes | orchestrator | [0200](adr/0200-cluster-topology.md) | Apache-2.0 | CNCF, graduated | Nomad | Nomad, Docker Swarm, systemd units, a PaaS |
| Talos Linux | node OS | [0200](adr/0200-cluster-topology.md) | MPL-2.0 | Sidero Labs, single-vendor | Debian stable converged by provisioning | Flatcar, Fedora CoreOS, Bottlerocket, Debian + Ansible |
| Cilium | CNI, network policy, east-west encryption | [0206](adr/0206-cluster-networking.md) | Apache-2.0 | CNCF, graduated | Calico in its eBPF dataplane | flannel, Calico, flannel + KubeSpan, flannel + Linkerd |
| SeaweedFS | object storage | [0207](adr/0207-cluster-storage.md) | Apache-2.0 | maintainer-led, small core team | any S3-compatible store behind the same client | MinIO, Garage, Ceph/Rook, a managed bucket |
| Argo CD | GitOps reconciliation | [0201](adr/0201-gitops.md) | Apache-2.0 | CNCF, graduated | Flux | Flux, plain CI apply, Kustomize overlays, Helm rendered then post-rendered |
| Temporal | durable execution | [0302](adr/0302-temporal.md) | MIT | Temporal Technologies, single-vendor | Restate | Cadence, Restate, Inngest, DBOS, plain queues, Conductor |
| Ory Kratos | identity and authentication | [0304](adr/0304-identity-and-authorization.md) | Apache-2.0 | Ory Corp, single-vendor | Zitadel | Zitadel, Keycloak, Authentik, SuperTokens, build it |
| Ory Hydra | OAuth2 / OIDC provider | [0304](adr/0304-identity-and-authorization.md) | Apache-2.0 | Ory Corp, single-vendor | Zitadel | Zitadel, Keycloak, Authentik, dex |
| OpenFGA | authorization (ReBAC) | [0304](adr/0304-identity-and-authorization.md) | Apache-2.0 | CNCF | SpiceDB | SpiceDB, Casbin, Oso, Permify, RBAC in the database |
| Ory Oathkeeper | edge authorization | [0305](adr/0305-edge-auth-and-traffic-policy.md) | Apache-2.0 | Ory Corp, single-vendor | Envoy `ext_authz` | Envoy `ext_authz`, Traefik ForwardAuth to a first-party service, Pomerium, per-service middleware |
| Traefik | edge routing | [0305](adr/0305-edge-auth-and-traffic-policy.md) | MIT | Traefik Labs, single-vendor | Envoy Gateway | Envoy Gateway, ingress-nginx, HAProxy, Contour |
| Forgejo | forge and CI | [0102](adr/0102-source-control-and-ci.md) | GPL-3.0-or-later | Codeberg e.V., non-profit | Gitea | Gitea, GitLab CE, Gogs, a provider-hosted forge |
| OpenAPI + REST | service interface contract | [0303](adr/0303-api-contracts-and-lifecycle.md) | specification, not a tool | OpenAPI Initiative, Linux Foundation | gRPC plus a transcoding gateway | gRPC, Connect, GraphQL, tRPC, JSON-RPC |
| Copier | template propagation | [0106](adr/0106-dependency-updates.md) | MIT | maintainer-led | a git remote with periodic merges | Cookiecutter, a git remote, copy-once |

## Tier 2 — substitutable

A bounded swap behind a stable interface. The owning ADR carries a short comparison table on the question that decided it.

| Tool | Concern | Owning ADR | Licence | Governance | Recorded against |
| --- | --- | --- | --- | --- | --- |
| Helm | manifest templating | [0201](adr/0201-gitops.md) | Apache-2.0 | CNCF, graduated | Kustomize overlays, Helm + post-render, jsonnet, plain YAML |
| Kyverno | admission policy | [0104](adr/0104-supply-chain-security.md), [0203](adr/0203-policy-enforcement.md) | Apache-2.0 | CNCF | Gatekeeper/OPA, Validating Admission Policy, the CI lint layer alone |
| cert-manager | TLS certificate lifecycle | [0200](adr/0200-cluster-topology.md) | Apache-2.0 | CNCF, graduated | certbot in a CronJob, a private CA, manual issuance |
| SOPS | secret encryption | [0202](adr/0202-secrets.md) | MPL-2.0 | CNCF | Sealed Secrets, External Secrets Operator, Vault, git-crypt |
| age | encryption backend for SOPS | [0202](adr/0202-secrets.md) | BSD-3-Clause | maintainer-led | GPG, cloud KMS, Vault transit |
| sops-secrets-operator | in-cluster decryption | [0202](adr/0202-secrets.md) | Apache-2.0 | maintainer-led | an init container, a CI-side decrypt, External Secrets Operator |
| Terraform | infrastructure provisioning | [0200](adr/0200-cluster-topology.md) | BUSL-1.1 | HashiCorp / IBM, single-vendor | OpenTofu, Pulumi, Crossplane, provider CLIs |
| zot | image registry | [0105](adr/0105-image-registry.md) | Apache-2.0 | CNCF | CNCF Distribution, Harbor, Quay, a provider registry |
| cosign | artefact signing | [0104](adr/0104-supply-chain-security.md) | Apache-2.0 | OpenSSF / Sigstore | notation/Notary v2, GPG, keyless via public Fulcio |
| syft | SBOM generation | [0104](adr/0104-supply-chain-security.md) | Apache-2.0 | Anchore, single-vendor | Trivy's own SBOM output, cdxgen, the build system's |
| Trivy | vulnerability scanning | [0104](adr/0104-supply-chain-security.md) | Apache-2.0 | Aqua Security | Grype, Clair, Snyk, registry-side scanning |
| ogen | Go server and client codegen | [0303](adr/0303-api-contracts-and-lifecycle.md) | Apache-2.0 | maintainer-led | oapi-codegen, go-swagger, hand-written handlers |
| `openapi-typescript` + `openapi-fetch` | TypeScript client codegen | [0303](adr/0303-api-contracts-and-lifecycle.md) | MIT | maintainer-led | Kubb, orval, Hey API, a hand-written fetch wrapper |
| vacuum | OpenAPI linting | [0303](adr/0303-api-contracts-and-lifecycle.md) | MIT | maintainer-led | Spectral, Redocly CLI, no spec lint |
| oasdiff | breaking-change detection | [0303](adr/0303-api-contracts-and-lifecycle.md) | Apache-2.0 | maintainer-led | openapi-diff, Optic, review alone |
| Scalar | API reference rendering | [0303](adr/0303-api-contracts-and-lifecycle.md) | MIT | Scalar, single-vendor | Redoc, Swagger UI, Stoplight Elements |
| Prism | API mock | [0600](adr/0600-local-development-loop.md) | Apache-2.0 | Stoplight / SmartBear | Microcks, WireMock, `muonsoft/openapi-mock`, MSW |
| sqlc | typed queries from SQL | [0300](adr/0300-data.md) | MIT | maintainer-led | GORM, ent, sqlx, pgx directly |
| dbmate | schema migrations | [0300](adr/0300-data.md) | MIT | maintainer-led | Atlas, golang-migrate, goose, Flyway |
| sqruff | SQL linting | [0300](adr/0300-data.md) | Apache-2.0 | Quary, single-vendor | sqlfluff, SQLFluff-lite, no SQL lint |
| PgBouncer | connection pooling | [0300](adr/0300-data.md) | ISC | PostgreSQL community | pgcat, Odyssey, application-side pooling only |
| Grafana | signal UI | [0500](adr/0500-observability.md), [0501](adr/0501-operator-uis-and-dashboards.md) | AGPL-3.0 | Grafana Labs, single-vendor | Perses, SigNoz, OpenObserve, Coroot, Kibana |
| Prometheus | metrics store | [0500](adr/0500-observability.md) | Apache-2.0 | CNCF, graduated | VictoriaMetrics, Mimir on day one, InfluxDB |
| Loki | log store | [0500](adr/0500-observability.md) | AGPL-3.0 | Grafana Labs | Elasticsearch/OpenSearch, VictoriaLogs, ClickHouse |
| Tempo | trace store | [0500](adr/0500-observability.md) | AGPL-3.0 | Grafana Labs | Jaeger, SigNoz, Zipkin |
| Pyroscope | profile store | [0500](adr/0500-observability.md) | AGPL-3.0 | Grafana Labs | Parca, Polar Signals, no continuous profiling |
| Alloy | profile scraping | [0500](adr/0500-observability.md) | Apache-2.0 | Grafana Labs | the Pyroscope agent, the OTel Collector's profiling support |
| OpenTelemetry Collector | telemetry pipeline | [0500](adr/0500-observability.md) | Apache-2.0 | CNCF | Fluent Bit, Vector, direct export from each service |
| Grafana Faro | browser telemetry | [0400](adr/0400-frontend.md), [0700](adr/0700-analytics.md) | Apache-2.0 | Grafana Labs | OTel browser SDK directly, Sentry's browser SDK, a hand-rolled beacon |
| Alertmanager | alert routing | [0502](adr/0502-alerting-and-on-call.md) | Apache-2.0 | Prometheus / CNCF | Grafana's built-in alerting, Karma, a webhook-only receiver |
| Headlamp | Kubernetes debug UI | [0501](adr/0501-operator-uis-and-dashboards.md) | Apache-2.0 | CNCF | Kubernetes Dashboard, Lens, k9s, `kubectl` alone |
| Hubble UI | flow visibility | [0501](adr/0501-operator-uis-and-dashboards.md) | Apache-2.0 | CNCF, ships with Cilium | the Hubble CLI alone, a service mesh's own map |
| Lowdefy | internal admin | [0401](adr/0401-internal-admin.md) | Apache-2.0 | maintainer-led, smallest community on the floor | Retool self-hosted, Appsmith, Budibase, Django-admin-style scaffolding, the `(admin)` route group |
| pgweb | read-only database inspector | [0401](adr/0401-internal-admin.md) | MIT | maintainer-led | pgAdmin, CloudBeaver, `psql` over a port-forward |
| maddy | outbound mail | [0307](adr/0307-outbound-email.md) | GPL-3.0 | maintainer-led | Postfix, Exim, Haraka, a transactional provider |
| Mailpit | non-production mail sink | [0307](adr/0307-outbound-email.md), [0601](adr/0601-testing-strategy.md) | MIT | maintainer-led | MailHog, smtp4dev, a logging sink |
| Playwright | browser testing | [0601](adr/0601-testing-strategy.md) | Apache-2.0 | Microsoft | Cypress, Selenium, Puppeteer |
| k6 | load testing | [0601](adr/0601-testing-strategy.md) | AGPL-3.0 | Grafana Labs | Gatling, Locust, JMeter, Vegeta |
| Testcontainers | integration test fixtures | [0601](adr/0601-testing-strategy.md) | MIT | AtomicJar / Docker | a shared test database, `cluster:base`, in-memory fakes |
| Lighthouse CI | frontend performance budget | [0400](adr/0400-frontend.md) | Apache-2.0 | Google | WebPageTest, Calibre, bundle-size checks alone |
| kind | inner-loop local cluster | [0600](adr/0600-local-development-loop.md) | Apache-2.0 | Kubernetes SIGs | Talos in Docker, k3d, minikube, Docker Desktop and Orbstack Kubernetes, Docker Compose, a remote dev cluster |
| talosctl | full-tier local cluster, provisioned from the machine config [ADR-0200](adr/0200-cluster-topology.md) decides | [0600](adr/0600-local-development-loop.md) | MPL-2.0 | Sidero Labs, single-vendor | k3d, kind, minikube, Docker Desktop and Orbstack Kubernetes for the full tier |
| mise | task runner and tool pinning | [0101](adr/0101-monorepo.md) | MIT | maintainer-led | Make, Task, `just`, asdf, Nix |
| golangci-lint | Go linting | [0101](adr/0101-monorepo.md) | GPL-3.0 | maintainer-led | `go vet` alone, revive, staticcheck standalone |
| Biome | TypeScript lint and format | [0101](adr/0101-monorepo.md) | MIT | maintainer-led | ESLint + Prettier, oxlint, dprint |
| rumdl | Markdown lint and format | [0101](adr/0101-monorepo.md) | MIT | maintainer-led | markdownlint, Vale, Prettier for Markdown |
| gitleaks | plaintext-secret scanning | [0202](adr/0202-secrets.md) | MIT | gitleaks organisation, maintainer-led | trufflehog, detect-secrets, review alone |
| shellcheck + shfmt | shell lint and format | [0101](adr/0101-monorepo.md) | GPL-3.0 and BSD-3-Clause | maintainer-led | no shell lint, shellharden |
| lefthook | git hooks | [0101](adr/0101-monorepo.md) | MIT | Evil Martians | husky, pre-commit, a committed `.githooks` directory |
| cocogitto | Conventional Commits and CalVer | [0103](adr/0103-release-and-versioning.md) | MIT | maintainer-led | commitlint, semantic-release, release-please, git-cliff |
| act | local workflow execution | [0102](adr/0102-source-control-and-ci.md) | MIT | nektos, maintainer-led | pushing to a branch, a self-hosted runner locally |
| Renovate | dependency updates | [0106](adr/0106-dependency-updates.md) | AGPL-3.0 | Mend, single-vendor | Dependabot, per-ecosystem updaters, manual sweeps |
| BuildKit | container image builds | [0102](adr/0102-source-control-and-ci.md) | Apache-2.0 | Moby | Buildah, Kaniko, a socket-mounted Docker daemon |
| Mimir | metrics at scale, Scale tier | [0500](adr/0500-observability.md) | AGPL-3.0 | Grafana Labs | Thanos, Cortex, VictoriaMetrics cluster |
| ClickHouse | analytics store, Scale tier | [0700](adr/0700-analytics.md) | Apache-2.0 | ClickHouse Inc. | DuckDB, TimescaleDB, staying on Postgres |
| GlitchTip | error tracking, Scale tier | [0503](adr/0503-error-tracking.md) | MIT | maintainer-led | Sentry self-hosted, Bugsink, fingerprints in Loki |
| Longhorn | replicated block storage, Scale tier | [0207](adr/0207-cluster-storage.md) | Apache-2.0 | CNCF | OpenEBS, Rook-Ceph, provider block storage |
| Dependency-Track | continuous SBOM triage, Scale tier | [0104](adr/0104-supply-chain-security.md) | Apache-2.0 | OWASP | Grype on a schedule, a registry's own scanner, Trivy alone |
| k6-operator | distributed load generation, Scale tier | [0601](adr/0601-testing-strategy.md) | AGPL-3.0 | Grafana Labs | k6 on a larger machine, a hosted load service |

## Tier 3 — libraries

Removal is a mechanical edit inside the packages that import it. Each row names what it was picked over; no ADR table is owed.

### Go

| Library | Concern | Picked over |
| --- | --- | --- |
| `pgx` | Postgres driver | `lib/pq`, `database/sql` with a generic driver |
| `otelpgx` | query tracing | hand-written span wrapping |
| OpenTelemetry Go SDK and contrib bridges | instrumentation | vendor SDKs, a hand-rolled metrics package |
| Temporal Go SDK | workflow client | the HTTP API directly |
| `openfga/go-sdk` | authorization client | raw HTTP against the OpenFGA API |
| `testify` | assertions | standard-library assertions, `gocheck`, `gomega` |
| `go-faster/jx` | JSON, as ogen's dependency | `encoding/json`, `jsoniter` |
| `go-faster/errors` | error wrapping, as ogen's dependency | `fmt.Errorf`, `pkg/errors` |
| `google/uuid` | UUID generation | `gofrs/uuid`, `oklog/ulid` |
| `math/big` | money arithmetic | `shopspring/decimal`, `govalues/decimal`, integer minor units |
| `ruleguard/dsl` | custom static-analysis rules | hand-written `go/analysis` passes |
| `yaml.v3` | YAML parsing | `ghodss/yaml`, `goccy/go-yaml` |

### Frontend

| Library | Concern | Picked over |
| --- | --- | --- |
| React 19 | UI runtime | Vue, Svelte, Solid |
| TanStack Query | server-state cache | SWR, Redux Toolkit Query, `use` with Suspense alone |
| `react-hook-form` + `@hookform/resolvers` | form state | Formik, React Final Form, uncontrolled forms |
| `zod` | runtime schema validation | Yup, Valibot, io-ts, ArkType |
| `zustand` | client state | Redux Toolkit, Jotai, Context alone |
| `nuqs` | URL-backed state | hand-written `searchParams` parsing |
| `next-themes` | theme switching | a hand-rolled class toggle |
| `tailwind-merge` | class conflict resolution | `clsx` alone, `cva` alone |
| `react-aria-components` | accessible primitives | Radix, Headless UI, hand-written ARIA |
| `@untitledui/icons` | icon set | Lucide, Heroicons, Phosphor |
| `tailwindcss-animate` | animation utilities | Framer Motion, hand-written keyframes |
| `tailwindcss-react-aria-components` | state variants for the primitives | manual data-attribute selectors |
| `@tailwindcss/typography` | prose styling | hand-written prose rules |
| `openapi-fetch` | typed client transport | `axios`, bare `fetch`, `ky` |
| `server-only` | build-time server-boundary guard | review alone |
| `pino` | structured logging | `winston`, `bunyan`, `console` |
| `@openfeature/web-sdk` | feature flags | a bespoke flag hook, a vendor SDK |
| `@scalar/api-reference-react` | in-app API reference | an iframe to a hosted renderer |
| `@grafana/faro-*` | browser telemetry transport | the OTel browser SDK directly |

## What is not here

**Transitive dependencies.** A package pulled in by something above is governed by the row above it. Depending on one directly makes it a Tier 3 adoption and gives it a row ([ADR-0002](adr/0002-tool-adoption.md)).

**Specifications.** OpenAPI 3.1, OCI, RFC 9457, RFC 9562, SLSA, and the ASVS are contracts this platform conforms to, not tools it runs. Where one was chosen over an alternative, the comparison is in the ADR that adopted it.

**Anything rejected.** A tool that lost appears in its winner's *Recorded against* column and in the owning ADR's *Considered options*, which is where a rejection stays visible. A rejection nobody can see is indistinguishable from an option nobody considered.
