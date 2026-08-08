# ADR-0003: Naming & Identifiers

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0200](0200-cluster-topology.md), [ADR-0202](0202-secrets.md)

## Context

**One instance of this repo is one project.** The template is instantiated once per project, so the project name is the identity of everything the instance owns. A project is operated independently and may run on any provider — broader than [ADR-0200](0200-cluster-topology.md), which documents the default one.

Every project produces a long tail of named things an engineer reads while switching between several projects in the same day: compute instances, provider accounts, object-storage buckets, identity principals, DNS zones, Kubernetes contexts and namespaces, node hostnames, SSH host aliases, and `age` recipients in `.sops.yaml` ([ADR-0202](0202-secrets.md)).

Two failures recur when this is left to taste:

| Failure | Consequence |
| --- | --- |
| A name in our files does not match the name in the provider's console | An engineer cannot grep across the boundary, and operates on the wrong resource |
| Two projects collide | `prod-db` means nothing once more than one project exists |

## Decision drivers

1. **File and console parity.** A name written in the repo is the exact string the provider's console shows. No per-provider mangling.
2. **The project name is globally unique.** Each project picks a slug distinctive enough to stand alone in any provider's global namespace, so no org prefix is ever needed.
3. **One way to do things** ([ADR-0000](0000-platform-foundations.md), principle 5). One grammar, every resource, every provider.
4. **Mechanical, not conditional.** The scheme is applied without a judgement call per resource.

## Considered options

| Option | File–console parity | Collision resistance | Why not |
| --- | --- | --- | --- |
| **One slug, lowest-common-denominator charset** | exact | the project slug carries it | **Chosen** |
| Per-provider name variants | broken by construction | good | Every name needs a translation table, and the grep across the boundary fails |
| Org prefix on every name (`acme-corp-…`) | exact | good | Spends characters of a tight budget on a constant, and the project slug already discriminates |
| Descriptive per-resource suffixes on collision (`-eu`, `-2`) | exact | poor | A meaningful suffix is itself a guess that can collide again, and reintroduces the per-resource judgement this ADR removes |
| Generated opaque names (UUID, provider-assigned) | exact | total | Unreadable, so a wrong-context command is invisible — the failure this ADR exists to prevent |

## Decision

### The slug grammar

Every named resource derives from one dash-joined slug:

```text
{project}-{env}-{role}[-{n}]
```

| Segment | Source | Examples |
| --- | --- | --- |
| `project` | the globally-unique project slug | `acme`, `northwind` |
| `env` | environment per [ADR-0200](0200-cluster-topology.md) | `dev`, `stg`, `prod` |
| `role` | node role or service short-name | `cp` (control plane), `web`, `api`, `db`, `assets` |
| `n` | ordinal, only when several of a role exist | `1`, `2`, `3` |

Worked example for project `acme`:

| Thing | Name |
| --- | --- |
| Compute instance | `acme-prod-cp-1` |
| Provider account or project | `acme-prod` |
| Object-storage bucket | `acme-prod-assets` |
| Kubernetes context | `acme-prod` |
| Kubernetes namespace | `acme-prod`, or service-scoped per the in-cluster rule below |
| SSH host alias | `acme-prod-cp-1` |
| `age` recipient | `acme-prod` |

### The lowest-common-denominator character rule

The slug satisfies the **most restrictive** naming rule across the providers targeted, so the same string is legal everywhere.

| Property | Rule | Why |
| --- | --- | --- |
| Charset | `^[a-z][a-z0-9-]*$` | The intersection of common provider rules and DNS-1123 labels, which forbid uppercase, underscores, and leading digits |
| Length | ≤ 30 characters for the full slug | The tightest identifier cap among the targeted providers. This is why `role` values are short — `cp`, not `control-plane` |

A slug passing this rule is reused verbatim in every provider and every file. No per-provider variant is generated.

### The global-namespace backstop

Object-storage buckets and provider account IDs live in a namespace shared with every customer of the provider. A well-chosen project slug makes a true collision vanishingly rare, and that is the first line of defence.

When it fires, the fix is mechanical: append a random token to the **project segment** and use that as the project slug everywhere.

```text
acme            → acme-prod-assets bucket taken globally
acme-x7q2       ← project slug becomes acme-x7q2, applied across all resources
```

The token is four lowercase-alphanumeric characters, generated once at instantiation and recorded as the project slug. It is not a per-resource suffix: the whole project carries it, so every derived name stays consistent and the grammar is unchanged. It is random rather than descriptive for the reason in *Considered options*.

### In-cluster names drop implied segments

Inside a single cluster there is exactly one project and one environment, so node hostnames and namespaces omit `{project}-{env}` where the context implies them — a node is `cp-1`, a service namespace is its service name.

The full slug is required only where names **cross the cluster boundary**: provider resources, Kubernetes *context* names, SSH aliases, and `age` recipients. The operator already selected the context to be inside the cluster; repeating the project on every in-cluster object is noise.

### SSH keys

| Concern | Rule |
| --- | --- |
| Key pair on a laptop | `~/.ssh/{project}-{env}`, e.g. `~/.ssh/acme-prod`. No owner segment — a laptop holds only its own engineer's key |
| `~/.ssh/config` host alias | the full instance slug, so `ssh acme-prod-cp-1` works without flags and reads the same as the console |
| Public keys in a shared namespace | gain a `{handle}` owner segment: `{project}-{env}-{handle}`, e.g. `acme-prod-alice` |
| Public key comment | `{project}-{env}-{handle} {date}` |
| Rotation | edit the per-project authorized keys and re-run the provisioning play ([ADR-0200](0200-cluster-topology.md)) |

Shared namespaces are the host's `authorized_keys`, a provider's named SSH-key resource, and the per-engineer `age` recipients of [ADR-0202](0202-secrets.md). Without the handle, every engineer's key reads as `{project}-{env}` in the one place they coexist and none traces to a person — so revoking the right one becomes guesswork. `handle` is a short, stable, lowercase per-engineer token obeying the same charset rule.

## Consequences

### Positive

- A name copied from a provider's console greps cleanly against the repo, and the reverse.
- Switching projects is safe: every name leads with the project, so a wrong-context command is visually obvious.
- One mechanical rule removes both the per-resource naming debate and the per-provider translation table.
- Dropping implied segments in-cluster keeps day-to-day names short without losing cross-boundary disambiguation.

### Negative / Risks

- The 30-character cap forces terse `role` tokens, which need a shared glossary.
- A long project name eats the budget. Projects whose natural name exceeds it pick a documented short slug at instantiation.
- The project slug carries the whole uniqueness guarantee and must be chosen carefully. The random-token backstop covers a true provider-global collision at the cost of five characters and some readability, so it is a fallback rather than the default.

### Follow-ups

- Template instantiation prompts for the project slug, validates it against the charset and length rules, checks it is not already taken, offers the random token when a provider-global name is unavailable, and threads the final slug through the SSH and age scaffolding, the provisioning inventory, and the infrastructure variables.
- A `role` abbreviation glossary kept alongside this ADR.

## Rules

- Every named resource derives from `{project}-{env}-{role}[-{n}]`. Shared infrastructure not tied to a product — a team proxy, an internal forge, a registry mirror — is modelled as its own project with its own slug and follows the same grammar. `(review-only)`
- The slug matches `^[a-z][a-z0-9-]*$` and is at most 30 characters, and the same string is used in files and in every provider's console without modification. `(review-only)`
- The project slug is globally unique and stands alone. No org or cross-project prefix is prepended. `(review-only)`
- A true provider-global collision is resolved by appending a random `[a-z0-9]{4}` token to the project slug, never a descriptive or per-resource suffix. `(review-only)`
- The full slug is required where names cross the cluster boundary — provider resources, Kubernetes contexts, SSH aliases, `age` recipients. In-cluster hostnames and namespaces drop the implied `{project}-{env}`. `(review-only)`
- SSH key pairs are named `{project}-{env}` on an engineer's laptop. Where several engineers' public keys share a namespace, each carries a `{handle}` owner segment. `(review-only)`
