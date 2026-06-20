# ADR-0015: Naming & Identifiers

- **Status:** Accepted
- **Date:** 2026-05-26
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0003](0003-cluster-topology.md), [ADR-0005](0005-secrets.md)

## Context

**One instance of this repo is one project.** The template is instantiated once per project; the *project name* is
therefore the identity of everything the instance owns. A project is built and operated independently and may run on
any cloud provider — broader than [ADR-0003](0003-cluster-topology.md), which documents the default provider. Every
project produces a long tail of named things that an engineer reads while switching between *several projects in the
same day*:

- Cloud-provider resources — compute instances, accounts/projects, object-storage buckets, identity principals, DNS
  zones.
- Kubernetes contexts, namespaces, and node hostnames.
- SSH host aliases and the keys behind them.
- `age` recipients in `.sops.yaml` ([ADR-0005](0005-secrets.md)).

Two failures recur when this is left to taste:

1. **A name in our files does not match the name in the provider's console**, so an engineer can't grep across the
   boundary and operates on the wrong resource.
2. **Two projects collide** — `prod-db` means nothing once you operate more than one project.

We need one identifier grammar, framed around the project name, that satisfies any cloud provider's naming rules and
reads identically in files and in every console.

## Decision drivers

1. **File ↔ console parity.** A name written in the repo is the exact string shown in the cloud provider's console.
   No per-provider mangling.
2. **The project name is globally unique.** Each project picks a slug that is unique across every project we run — and
   chosen to be distinctive enough to stand alone in any provider's global namespace. Because the name itself is the
   discriminator, no org prefix is ever needed.
3. **One way to do things** ([ADR-0000](0000-platform-foundations.md)). One slug grammar, every resource, every cloud.
4. **Unambiguous, not conditional** ([ADR-0000](0000-platform-foundations.md)). The scheme is mechanical, not a
   judgement call per resource.

## Decisions

### The slug grammar

Every named resource derives from a single dash-joined slug:

```text
{project}-{env}-{role}[-{n}]
```

| Segment   | Source                                               | Examples                                           |
|-----------|------------------------------------------------------|----------------------------------------------------|
| `project` | the globally-unique project slug                     | `acme`, `northwind`                                |
| `env`     | environment per [ADR-0003](0003-cluster-topology.md) | `dev`, `stg`, `prod`                               |
| `role`    | node role or service short-name                      | `cp` (control plane), `web`, `api`, `db`, `assets` |
| `n`       | ordinal, **only** when several of a role exist       | `1`, `2`, `3`                                      |

Worked example for project `acme`:

| Thing                            | Name                                                          |
|----------------------------------|---------------------------------------------------------------|
| Compute instance                 | `acme-prod-cp-1`                                              |
| Cloud account / project          | `acme-prod`                                                   |
| Object-storage bucket            | `acme-prod-assets`                                            |
| Kubernetes context               | `acme-prod`                                                   |
| Kubernetes namespace             | `acme-prod` (or service-scoped per the in-cluster note below) |
| SSH host alias (`~/.ssh/config`) | `acme-prod-cp-1`                                              |
| `age` recipient name             | `acme-prod`                                                   |

### The lowest-common-denominator character rule

The slug MUST satisfy the **most restrictive** naming rule across the providers we target, so the same string is
legal everywhere:

- **Charset:** `^[a-z][a-z0-9-]*$` — lowercase letters, digits, hyphens; starts with a letter. This is the
  intersection of common provider rules and DNS-1123 labels, which forbid uppercase, underscores, and leading digits.
- **Length:** keep the full slug **≤ 30 characters** — the tightest identifier cap among the providers we target. This
  is why `role` values are short (`cp`, not `control-plane`).

A slug that passes this rule is **reused verbatim** for the resource in every provider and every file. We do not
generate a per-provider variant.

### The global-namespace backstop

A few resource types — object-storage buckets, cloud account/project IDs — live in a namespace shared with *every*
customer of the provider, not just our projects. A well-chosen project slug makes a true collision here vanishingly
rare, and that is the first line of defence: **pick a distinctive enough slug that this never fires.**

When it does fire, the fix is mechanical, not a judgement call: append a short random token to the **project
segment** and use that as the project slug everywhere.

```text
acme            → acme-prod-assets bucket taken globally
acme-x7q2       ← project slug becomes acme-x7q2, applied across all resources
```

- The token is **4 lowercase-alphanumeric characters** (`[a-z0-9]{4}`), generated once at instantiation and recorded
  as the project slug. It is not a per-resource suffix — the whole project carries it, so every derived name stays
  consistent and the grammar is unchanged.
- Random, not descriptive: a meaningful suffix (`-eu`) is itself a guess that can collide again and reintroduces the
  per-resource judgement this ADR exists to avoid.

### In-cluster names drop redundant segments

Inside a single cluster there is exactly one project and one environment, so node hostnames and namespaces MAY omit
`{project}-{env}` where the context already implies them — a node is simply `cp-1`, a service namespace is its
service name. The **full slug is mandatory only where names cross the cluster boundary**: cloud-provider resources,
kube *context* names, SSH aliases, and `age` recipients. The operator already selected the context to be inside the
cluster; repeating the project on every in-cluster object is noise.

### SSH keys

- **Key naming:** one key pair per engineer per project: `~/.ssh/{project}-{env}` (e.g. `~/.ssh/acme-prod`). The
  matching `~/.ssh/config` `Host` alias is the full instance slug (`acme-prod-cp-1`), so `ssh acme-prod-cp-1` works
  without flags and reads the same as the panel.
- **Rotation:** keys are rotated on engineer offboarding or suspected compromise via the same Ansible run that manages
  `authorized_keys` ([ADR-0003](0003-cluster-topology.md)). Authorized public keys are tracked in the repo per project;
  removing one and re-running the playbook is the revocation procedure.

## Consequences

### Positive

- A name copied from a provider's console greps cleanly against the repo, and vice versa.
- Switching projects is safe: every name leads with the project, so a wrong-context command is visually obvious.
- One mechanical rule means no per-resource naming debate and no per-provider translation table.
- Dropping redundant segments in-cluster keeps day-to-day names short without losing cross-boundary disambiguation.

### Negative / Risks

- The ≤30-char cap forces terse `role` tokens; the role abbreviations need a shared glossary (kept with this ADR).
- Long project names eat into the budget; projects whose natural name exceeds the budget pick a documented short slug
  at instantiation.
- The project slug must be picked carefully at instantiation: it has to be globally distinctive, since it carries the
  whole uniqueness guarantee. The random-token backstop covers a true provider-global collision but costs 5 characters
  of the 30-char budget and makes the slug less readable, so it is a fallback, not the default.

### Follow-ups

- Template instantiation (`scripts/`/`mise` bootstrap): prompt for the project slug, validate it against
  `^[a-z][a-z0-9-]*$` and the 30-char budget, check it is not already taken by another of our projects, offer to
  append a random `[a-z0-9]{4}` token if a provider-global resource name is unavailable, and thread the final slug
  through the SSH/age scaffolding and the Ansible inventory (and Terraform variables when the project provisions its own
  infra).
- A `role` abbreviation glossary kept alongside this ADR.

## Rules

- Every named resource derives from `{project}-{env}-{role}[-{n}]`; shared infrastructure not tied to a product (team proxy, internal GitLab, registry mirror) is modelled as its own project with a distinct slug (e.g. `platform`, `gitlab-internal`) and follows the same grammar.
- The slug matches `^[a-z][a-z0-9-]*$` and is ≤ 30 characters; the same string is used in files and in every
  provider's console without modification.
- The project slug is globally unique and stands alone; no org or cross-project prefix is ever prepended.
- A true provider-global collision (buckets, account IDs) is resolved by appending a random `[a-z0-9]{4}` token to the
  project slug, never a descriptive or per-resource suffix.
- The full slug is required where names cross the cluster boundary (cloud-provider resources, kube contexts, SSH
  aliases, `age`
  recipients); in-cluster hostnames and namespaces MAY drop the implied `{project}-{env}`.
- SSH key pairs are named `{project}-{env}`; rotation is an Ansible re-run after editing the per-project authorized
  keys.
