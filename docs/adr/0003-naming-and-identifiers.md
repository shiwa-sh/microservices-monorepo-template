# ADR-0003: Naming & Identifiers

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0001](0001-documentation-and-output-conventions.md), [ADR-0200](0200-cluster-topology.md), [ADR-0202](0202-secrets.md), [ADR-0300](0300-data.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0500](0500-observability.md)
- **Decides:** Every named resource derives from one dash-joined `{project}-{env}-{role}` slug, and every entity identifier is a UUIDv7 carried on the wire with a type prefix.

## Context

**One instance of this repo is one project.** The template is instantiated once per project, so the project name is the identity of everything the instance owns. A project is operated independently and **may run on any provider** — broader than [ADR-0200](0200-cluster-topology.md), which documents the default topology rather than a provider.

Two families of name run through the platform, and both fail the same way when left to taste.

**Resource names** are read by an engineer switching between projects in a day: compute instances, provider accounts, object-storage buckets, DNS zones, Kubernetes contexts and namespaces, node hostnames, SSH host aliases, and `age` recipients in `.sops.yaml` ([ADR-0202](0202-secrets.md)).

**Entity identifiers** are the values a service assigns to its own rows and hands to callers: primary keys, the ID in a URL path, the value in a support ticket, the key that makes a retry safe.

| Failure | Consequence |
| --- | --- |
| A name in our files does not match the name in the provider's console | An engineer cannot grep across the boundary, and operates on the wrong resource |
| Two projects collide | `prod-db` means nothing once more than one project exists |
| Two surfaces spell the same environment differently | Every join between them needs a translation, and one of them is stale |
| An identifier does not say what it identifies | A value in a log or a ticket is untraceable, and a wrong-type argument compiles |
| Identifier choice is left to the first service that needs one | The second service chooses differently, and the pair can never share tooling |

This ADR owns the **grammar and charset** of every name the platform assigns. Where a surface's naming is a decision in its own right — URL paths ([ADR-0303](0303-api-contracts-and-lifecycle.md)), hostnames ([ADR-0306](0306-trust-tiers-and-urls.md)), task names ([ADR-0101](0101-monorepo.md)), metric names ([ADR-0500](0500-observability.md)), release tags ([ADR-0103](0103-release-and-versioning.md)) — that ADR owns it and this one links.

## Decision drivers

1. **File and console parity.** A name written in the repo is the exact string the provider's console shows.
2. **Provider agnostic.** The scheme rests on published standards, not on a survey of one provider set. A project adopting this template may run anywhere.
3. **The project name is globally unique.** Each project picks a slug distinctive enough to stand alone in any provider's global namespace, so no org prefix is ever needed.
4. **Mechanical, not conditional.** Applied without a judgement call per resource ([ADR-0000](0000-platform-foundations.md), principle 5). A vocabulary this ADR does not close is a vocabulary the next engineer invents.
5. **Adopt, don't invent** ([ADR-0001](0001-documentation-and-output-conventions.md)). Every charset, cap, and identifier format below is a published standard or is derived from one in the open.

## Considered options

### Resource names

| Option | File–console parity | Provider agnostic | Verdict |
| --- | --- | --- | --- |
| **One slug on a standards-derived charset, plus one mechanical compact form** | exact | yes — the charset is RFC 1123, not a provider survey | **Chosen** |
| One slug on the intersection of a named provider set | exact | no — the intersection changes when the provider set does, and a template cannot name its provider set | Re-derivable only by re-surveying providers, which nobody does. The cap becomes folklore |
| Per-provider name variants | broken by construction | yes | Every name needs a translation table, and the grep across the boundary fails |
| Org prefix on every name (`acme-corp-…`) | exact | yes | Spends characters of a tight budget on a constant, and the project slug already discriminates |
| Descriptive per-resource suffixes on collision (`-eu`, `-2`) | exact | yes | A meaningful suffix is itself a guess that can collide again, and reintroduces the per-resource judgement this ADR removes |
| Generated opaque names (UUID, provider-assigned) | exact | yes | Unreadable, so a wrong-context command is invisible — the failure this ADR exists to prevent |

### Entity identifiers

Exit cost is months and customer data: an identifier is embedded in every row, every URL a customer has bookmarked, and every log line ever written. This comparison is the deep one.

| Option | Enumeration exposure | Index behaviour | Type safety | Verdict |
| --- | --- | --- | --- | --- |
| **UUIDv7 stored, type-prefixed on the wire** | none — 74 random bits per value | sequential prefix, so inserts append | the prefix names the type at every boundary | **Chosen.** [RFC 9562](https://www.rfc-editor.org/info/rfc9562/) for the value, the Stripe/[TypeID](https://github.com/jetify-com/typeid) convention for the surface |
| `bigserial` | total — count and creation order leak from any single ID | ideal | none | Two services cannot mint IDs independently, and a leaked count is a business fact we did not choose to publish |
| UUIDv4 | none | random, so every insert lands in a cold page and the B-tree fragments | none | The write-amplification is paid on every table forever, and v7 removes it at no cost |
| ULID | none | sequential | none | Solves the same problem as v7 outside the RFC. No database type, no `uuid` column, no library in every language |
| Bare UUIDv7, no prefix | none | sequential | none | Loses the whole reason the surface convention exists: a bare UUID in a log or a ticket says nothing about what it identifies |
| Separate internal key and public key | none | ideal | partial | Two identifiers per row, a join to translate, and a permanent question of which one a given code path holds |

## Decision

### The slug grammar

Every named resource derives from one dash-joined slug:

```text
{project}-{env}-{role}[-{n}]
```

| Segment | Source | Values |
| --- | --- | --- |
| `project` | the globally-unique project slug, fixed at instantiation | `northwind`, `acmeco` |
| `env` | the environment, verbatim from [ADR-0200](0200-cluster-topology.md) | `dev`, `staging`, `prod` — no abbreviations |
| `role` | one token from the closed table below | `cp`, `node`, `assets` |
| `n` | ordinal, only where several of a role exist | `1`, `2`, `3` |

The `env` token is the environment's own name and nothing else. It is the same string in the GitOps tree, in the Kubernetes context, and in the OTel `deployment.environment.name` attribute ([ADR-0500](0500-observability.md)), so no surface needs a mapping to another.

Worked example for project `northwind`:

| Thing | Name |
| --- | --- |
| Compute instance | `northwind-prod-cp-1` |
| Provider account or project | `northwind-prod` |
| Object-storage bucket | `northwind-prod-assets` |
| Kubernetes context | `northwind-prod` |
| Kubernetes namespace | `northwind-prod`, or service-scoped per the in-cluster rule below |
| SSH host alias | `northwind-prod-cp-1` |
| `age` recipient | `northwind-prod` |

### `role` is a closed vocabulary

Driver 4 is not satisfied by a scheme with one free-text field. `role` takes a value from this table and no other:

| `role` | Names |
| --- | --- |
| `cp` | a Kubernetes control-plane node |
| `node` | a Kubernetes worker node |
| `lb` | a load balancer |
| `net` | a network or VPC |
| `fw` | a firewall or security group |
| `dns` | a DNS zone |
| `assets` | the public object-storage bucket |
| `backup` | the backup object-storage bucket ([ADR-0200](0200-cluster-topology.md)) |
| `state` | the infrastructure-state bucket |

Adding a resource class adds a row here in the same PR that adds the resource. A token is at most six characters, which is why the table reads `cp` rather than `control-plane`.

### Charset and length, derived rather than surveyed

The slug rests on **[RFC 1123](https://www.rfc-editor.org/rfc/rfc1123) DNS labels**, the one identifier form that DNS, TLS names, Kubernetes objects, hostnames, and provider APIs converge on. Driver 2 is met because the anchor is a standard, so a project on an unfamiliar provider re-derives the rule instead of re-surveying.

| Property | Rule | Derivation |
| --- | --- | --- |
| Charset | `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` | An RFC 1123 DNS label — lowercase alphanumerics and interior hyphens — narrowed to a leading letter, which [RFC 1035](https://www.rfc-editor.org/rfc/rfc1035) surfaces such as a Kubernetes Service require. The pattern admits no trailing hyphen and no doubled hyphen, both of which a DNS label rejects |
| Full slug length | ≤ 63 characters | Four independent standards land on the same number: an RFC 1123 DNS label, a [Kubernetes object name and label value](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/), and a PostgreSQL identifier at `NAMEDATALEN - 1` ([Postgres lexical structure](https://www.postgresql.org/docs/current/sql-syntax-lexical.html)) |
| Project slug length | 6–11 characters, including any collision token | The upper bound is the compact-form budget below: `24 - len("staging") - 6`, where 6 is the `role` cap. The lower bound is the highest minimum seen on a globally-namespaced provider identifier |

A slug passing this rule is reused verbatim in every provider and every file.

### The compact form

Some namespaces reject hyphens and cap shorter than a DNS label — a storage-account-class name is the common case, at 24 characters of lowercase alphanumerics. The slug's **compact form** serves them: the canonical slug with hyphens removed.

```text
northwind-prod-assets   canonical, used everywhere that accepts a DNS label
northwindprodassets     compact, used only where hyphens are rejected
```

This is one pure function applied identically on every provider, not the per-provider translation table rejected in *Considered options*: there is nothing to look up, and either form is derivable from the other by eye. The project-slug cap is set so the compact form always fits 24 characters. Ordinals are excluded from that budget because a role taking an ordinal is a node or an instance, which never lands in a flattened namespace.

### Names carry identity, labels carry provenance

A name is short because the namespaces it must satisfy are short. Everything a name cannot afford is attached beside it, which is the split both the [Azure Cloud Adoption Framework](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-naming) and Kubernetes make.

| Fact | Where it lives |
| --- | --- |
| Identity — which resource this is | the slug |
| Application, component, part-of, managed-by | [Kubernetes recommended labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/), `app.kubernetes.io/*`, already applied by [ADR-0201](0201-gitops.md) |
| Project and environment, in telemetry | OTel `service.namespace` and `deployment.environment.name` ([ADR-0500](0500-observability.md)) |
| Anything else — owner, cost centre, ticket | a provider tag, never a name segment |

Adding a segment to a name is therefore a last resort: most provider names are immutable, so the segment can never be corrected, while a label or a tag can.

### Only constant facts go in a name

A name records what will not change for the life of the resource. This is the Cloud Adoption Framework's rule, and [RFC 1178](https://www.rfc-editor.org/rfc/rfc1178.html) reaches it from the other direction — it lists naming a machine after its function among the practices to avoid, because the function moves and the name does not.

`role` is a deliberate departure and the only one. A Kubernetes node's role is structural rather than functional: a control-plane node does not become a worker, and a machine whose purpose changes is replaced rather than renamed. Application workloads, whose placement does move, are not named by this grammar at all — they are Kubernetes objects carrying `app.kubernetes.io/*` labels.

### In-cluster names drop implied segments

Inside a single cluster there is exactly one project and one environment, so node hostnames and namespaces omit `{project}-{env}` — a node is `cp-1`, a service namespace is its service name. The dropped segments are recoverable from the labels and resource attributes above; they are omitted, not lost.

The full slug is required only where names **cross the cluster boundary**: provider resources, Kubernetes *context* names, SSH aliases, and `age` recipients. The operator already selected the context to be inside the cluster; repeating the project on every in-cluster object is noise.

### The global-namespace backstop

Object-storage buckets and provider account identifiers live in a namespace shared with every other customer of that provider. Two defences, in order.

The first is **availability checked at instantiation**, before the slug is committed to anything. A well-chosen project slug makes a later collision rare, and a collision found at instantiation costs one prompt.

The second fires when a collision appears anyway, and is mechanical: append a random `[a-z0-9]{4}` token to the **project segment** and use the result as the project slug everywhere.

```text
northwind       → northwind-prod-assets unavailable in a global namespace
nwind7q2        ← project slug becomes nwind7q2, applied across all resources
```

The token is generated once and recorded as the project slug, inside the 6–11 character budget. It is not a per-resource suffix: the whole project carries it, so every derived name stays consistent and the grammar is unchanged. It is random rather than descriptive for the reason in *Considered options*.

### A second cluster in one environment

One environment is one cluster ([ADR-0200](0200-cluster-topology.md)). A second cluster in the same environment — a second region, a residency boundary — needs a discriminator the grammar has no segment for.

| Field | Value |
| --- | --- |
| **Trigger** | a second cluster provisioned inside one environment |
| **Seam** | yes. The second cluster is modelled as its own project with its own slug, which the Rules already require of any infrastructure not tied to a single product. No new segment, no renaming |
| **Cost if adopted late** | none structural. The two clusters carry unrelated project slugs, so cross-cluster tooling identifies them by their `service.namespace` attribute rather than by a shared prefix |

### SSH keys

| Concern | Rule |
| --- | --- |
| Key pair on a laptop | `~/.ssh/{project}-{env}`, e.g. `~/.ssh/northwind-prod`. No owner segment — a laptop holds only its own engineer's key |
| `~/.ssh/config` host alias | the full instance slug, so `ssh northwind-prod-cp-1` works without flags and reads the same as the console |
| Public keys in a shared namespace | gain a `{handle}` owner segment: `{project}-{env}-{handle}`, e.g. `northwind-prod-alice` |
| Public key comment | `{project}-{env}-{handle} {date}` |
| Rotation | edit the per-project authorized keys and re-run the provisioning play ([ADR-0200](0200-cluster-topology.md)) |

Shared namespaces are the host's `authorized_keys`, a provider's named SSH-key resource, and the per-engineer `age` recipients of [ADR-0202](0202-secrets.md). Without the handle, every engineer's key reads as `{project}-{env}` in the one place they coexist and none traces to a person — so revoking the right one becomes guesswork. `handle` is a short, stable, lowercase per-engineer token obeying the same charset rule.

### Entity identifiers

**Every row's primary key is a UUIDv7**, standardised in [RFC 9562](https://www.rfc-editor.org/info/rfc9562/). It is stored in a Postgres `uuid` column and generated in application code, so a service knows the identifier before the insert and does not depend on a database version for the generator.

**Every identifier crossing a service boundary is type-prefixed**, following the [TypeID](https://github.com/jetify-com/typeid) form:

```text
order_01j8xk7m3q0000000000000000
{prefix}_{UUIDv7 in 26 characters of base32}
```

| Property | Rule |
| --- | --- |
| Prefix | the singular `snake_case` form of the resource's collection noun in [ADR-0303](0303-api-contracts-and-lifecycle.md) — `/orders` yields `order_` |
| Where the prefixed form appears | request and response bodies, URL path parameters, logs, telemetry attributes, and error messages |
| Where the bare UUID appears | the `uuid` column, and nowhere else |
| Conversion | at the transport boundary, by one library per language ([ADR-0100](0100-language-and-runtime.md)) |
| Client treatment | opaque. A consumer never parses, orders, or constructs an identifier — [AIP-122](https://google.aip.dev/122) |

The prefix pays for itself three times: a value in a support ticket says what it is, a log line is greppable by type, and a generated client cannot pass an `order_` where a `product_` belongs.

**An unguessable identifier is not an authorisation control.** [OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Insecure_Direct_Object_Reference_Prevention_Cheat_Sheet.html) is explicit that a random identifier slows enumeration and prevents nothing else. Every read and every mutation is authorised by [ADR-0304](0304-identity-and-authorization.md) as though the identifier were public, because it is.

### Identifiers that are not entity identifiers

Three values are frequently confused with an entity ID. Each has one owner and one standard.

| Value | Form | Owner |
| --- | --- | --- |
| Correlation across services | the W3C [Trace Context](https://www.w3.org/TR/trace-context/) `traceparent`. No `X-Request-Id` is minted | [ADR-0500](0500-observability.md) |
| Client-supplied retry deduplication | the `Idempotency-Key` request header carrying a client-generated random value, per [draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/html/draft-ietf-httpapi-idempotency-key-header-07) | [ADR-0303](0303-api-contracts-and-lifecycle.md) |
| Server-side execution deduplication | the Temporal Workflow ID, derived from the business key rather than from the `Idempotency-Key` | [ADR-0302](0302-temporal.md) |

The last two are not alternatives. The header deduplicates a retried HTTP request at the edge of the service; the Workflow ID deduplicates the business operation however it was reached.

### Casing by surface

One table, so no surface is left to precedent. Where another ADR owns the naming decision, this row is the casing and that ADR is the rule.

| Surface | Form | Owner |
| --- | --- | --- |
| Provider resources, contexts, SSH aliases, `age` recipients | the slug grammar above | here |
| Kubernetes object names | RFC 1123 DNS label; RFC 1035 where the object requires a leading letter | here |
| Repository directories | `kebab-case` | here |
| SQL tables and columns | `snake_case`; plural table, singular column. Identifiers stay under 63 bytes | here, applied by [ADR-0300](0300-data.md) |
| JSON request and response fields | `snake_case`, noun-shaped, plural where repeated — [AIP-140](https://google.aip.dev/140) | here |
| OpenAPI schema names | `PascalCase`; `operationId` is `camelCase` | [ADR-0303](0303-api-contracts-and-lifecycle.md) |
| URL paths | plural resource nouns, no version segment | [ADR-0303](0303-api-contracts-and-lifecycle.md) |
| Hostnames | `{tool}.ops.<host>` and the trust-tier scheme | [ADR-0306](0306-trust-tiers-and-urls.md) |
| Environment variables | `SCREAMING_SNAKE_CASE` | here |
| Go and TypeScript identifiers | the language's own convention | [ADR-0001](0001-documentation-and-output-conventions.md) |
| `mise` tasks | `group:member` | [ADR-0101](0101-monorepo.md) |
| Metrics | OTel semantic conventions, else `<service>_<noun>_<unit>_<type>` | [ADR-0500](0500-observability.md) |
| Git tags, image tags, commit scopes | CalVer and the component path slug | [ADR-0103](0103-release-and-versioning.md) |

## Consequences

### Positive

- A name copied from a provider's console greps cleanly against the repo, and the reverse.
- Switching projects is safe: every name leads with the project, so a wrong-context command is visually obvious.
- The charset and the caps are derived from published standards, so a project on a provider nobody here has used re-derives them rather than guessing.
- One environment token spans the GitOps tree, the Kubernetes context, and telemetry, so no join between them needs a mapping.
- A closed `role` table and a single compact-form transform remove both the per-resource naming debate and the per-provider translation table.
- A prefixed identifier is self-describing in a log, a ticket, and a function signature, and UUIDv7 keeps the primary-key index append-ordered.

### Negative / Risks

- **The 24-character compact-form budget constrains the project slug to 11 characters** even for projects that will never touch a namespace that short. Accepted: the alternative is discovering the constraint after the names are immutable.
- The six-character `role` cap forces terse tokens. Mitigated by the table being closed and in this ADR rather than in a glossary elsewhere.
- A long project name eats the budget. Projects whose natural name exceeds it pick a documented short slug at instantiation.
- The project slug carries the whole uniqueness guarantee. The random-token backstop covers a true provider-global collision at the cost of readability, so it is a fallback rather than the default.
- **Prefixed identifiers change every API surface.** A spec's `type: string, format: uuid` becomes a pattern-constrained string, and the generated clients change with it. The encode/decode step is one library per language and is paid on every boundary crossing.
- Storing the bare UUID while serving the prefixed form means the two differ in a `psql` session. Accepted: the alternative is a `text` primary key, which costs more on every index than the conversion costs on every request.

## Rules

### Resource names

- Every named resource derives from `{project}-{env}-{role}[-{n}]`. Shared infrastructure not tied to a product — a team proxy, an internal forge, a registry mirror — is modelled as its own project with its own slug and follows the same grammar.
- A slug matches `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` and is at most 63 characters.
- The project slug is 6–11 characters including any collision token, is globally unique, and stands alone. No org or cross-project prefix is prepended.
- `env` is `dev`, `staging`, or `prod`, spelled the same way in every surface. Abbreviated forms are not used.
- `role` is a token from the table in this ADR. A new resource class adds a row in the same PR.
- The same string is used in files and in every provider's console. Where a namespace rejects hyphens, the compact form — the slug with hyphens removed — is used, and no other transformation is applied.
- A name records only facts that are constant for the life of the resource. Owner, cost centre, and application metadata are Kubernetes labels, OTel resource attributes, or provider tags.
- A true provider-global collision is resolved by appending a random `[a-z0-9]{4}` token to the project slug, never a descriptive or per-resource suffix.
- The full slug is required where names cross the cluster boundary — provider resources, Kubernetes contexts, SSH aliases, `age` recipients. In-cluster hostnames and namespaces drop the implied `{project}-{env}`.
- SSH key pairs are named `{project}-{env}` on an engineer's laptop. Where several engineers' public keys share a namespace, each carries a `{handle}` owner segment.

### Identifiers

- A primary key is a UUIDv7 in a Postgres `uuid` column, generated in application code. Sequential integer keys and UUIDv4 are not used. `(ref: RFC 9562)`
- An identifier crossing a service boundary is type-prefixed as `{prefix}_{base32 UUIDv7}`, where `prefix` is the singular form of the resource's collection noun. The bare UUID appears only in the database. `(ref: RFC 9562)`
- An identifier is opaque to its consumer. No client parses, orders, or constructs one.
- An unguessable identifier is never treated as an access control. Every access is authorised per [ADR-0304](0304-identity-and-authorization.md).
- Cross-service correlation uses the W3C `traceparent`. No service mints its own request identifier. `(ref: W3C Trace Context)`
- Client retry deduplication uses the `Idempotency-Key` request header. It is not reused as a Temporal Workflow ID.

### Casing

- Every surface uses the casing in the *Casing by surface* table. A surface absent from that table is added to it before it is used.
- JSON request and response fields are `snake_case` nouns, plural where repeated.
- SQL tables are plural `snake_case` and columns are singular `snake_case`, and every identifier stays under 63 bytes. `(CI: lint:sql)`
- Environment variables are `SCREAMING_SNAKE_CASE`.
