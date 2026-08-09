# ADR-0307: Outbound Email

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0302](0302-temporal.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0500](0500-observability.md)

## Context

Identity depends on mail. [ADR-0304](0304-identity-and-authorization.md)'s Kratos flows send address verification and account recovery, and neither has a fallback: an undelivered recovery mail is a locked-out user. Product notifications ride the same path, dispatched through the outbox or a workflow ([ADR-0302](0302-temporal.md)).

Outbound email is the component [ADR-0000](0000-platform-foundations.md) ranks **first** to concede when moving down axis B, because its principal cost is not operational but **reputational**. Standing the server up is a small task; being accepted is not, because acceptance is decided by receivers from the sending IP's history, the domain's authentication records, and complaint rates. Engineering effort cannot retire that cost, only accumulate it.

Two constraints shape every option. Many hosting providers block outbound port 25 by default. Residential and cloud IP ranges carry pre-existing reputation the platform inherits rather than earns.

**Scope: outbound only.** The platform sends mail. It does not receive it, host mailboxes, or run IMAP, so the inbound half of every mail suite is out of scope.

## Decision drivers

1. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). Mail leaves infrastructure we control, as the product's own domain.
2. **The reputation asset is the domain and the IP**, not the software. Novelty is priced accordingly (principle 4), because swapping the agent forfeits nothing that was earned.
3. **Thinnest viable platform** (principle 2). A submission agent, not a mail suite.
4. **Switching sender costs a configuration change.** [ADR-0000](0000-platform-foundations.md) concedes this component first, so whatever is chosen must not be reachable from application code.
5. **A silently dropped mail is visible as a metric**, not as a support ticket.

## Considered options

| Option | Sends as our domain | Component weight | What it costs | Verdict |
| --- | --- | --- | --- | --- |
| **maddy, submission and DKIM only** | yes | **one Go binary**, no datastore | we own IP warmup, blocklist monitoring, and DMARC reporting | **Chosen.** Holds principle 3 at the lowest component count, and the software is cheap to swap because the reputation lives elsewhere |
| Postfix | yes | one daemon, plus its own spool | the same reputation work, plus a configuration language of its own | The boring choice, and principle 4 says be boring where exit costs months. Exit cost here is a config rewrite, not a migration — the reputation is unaffected by which agent sends. Rejected on operational legibility, not on maturity |
| OpenSMTPD | yes | one daemon | the same reputation work | The direct answer to the legibility complaint above: its configuration language is the smallest of any full MTA. It loses narrowly on DKIM, which is a filter to wire up rather than a built-in, so the thing this ADR most needs is the one part not in the box |
| A relay-only client — msmtp, nullmailer, or Postfix as a null client | **no** — it relays through some other sender | none worth counting | none of the reputation work, because it does none of the sending | Not an alternative but a shape: it presumes the managed row below. Worth naming because "self-host the SMTP endpoint" and "own the deliverability" are separable, and only the second is expensive |
| Stalwart | yes | one binary, broader scope | an inbound, JMAP, and mailbox surface we do not use | Capable and young. Rejected for carrying the inbound half this ADR scopes out |
| Mailu / Mailcow | yes | a suite — several containers, a datastore, webmail, antispam | a full mail platform | Rejected by principle 2. These solve *running an email provider*, which is not the problem |
| Postal | yes | Ruby, MariaDB, RabbitMQ | a second datastore and a second message broker | Rejected by principles 2 and 5 |
| Managed transactional provider (Postmark, SES, Mailgun) | yes | none | deliverability becomes someone else's problem, and recipient metadata leaves our control | **The sanctioned exit**, ranked first on [ADR-0000](0000-platform-foundations.md)'s swap list. Not the default, because principle 3 is a constraint rather than a preference |
| A privacy-first mailbox host — Posteo, Mailbox.org | **no** | none | — | A different product category: these host human correspondence, do not carry custom sending domains, and offer no transactional API. Driver 1 requires mail to leave as the product's own domain, which rules the category out before any other criterion applies |
| Do nothing — the honest baseline | n/a | none | — | Account verification and recovery have no delivery path, so identity is unusable |

## Decision

| Concern | Decision |
| --- | --- |
| Agent | **maddy**, configured as a submission endpoint and DKIM signer. No inbound listener, no mailboxes |
| Egress | a **dedicated static IP** with a matching `PTR` record. Shared or dynamic egress is not used for mail |
| Authentication records | `SPF`, `DKIM`, and `DMARC` are committed alongside the environment's other DNS ([ADR-0200](0200-cluster-topology.md)). DMARC starts at `p=none` with reporting and moves to `p=reject` once reports are clean |
| Signing key | the DKIM private key is SOPS-encrypted ([ADR-0202](0202-secrets.md)) |
| Senders | Kratos ([ADR-0304](0304-identity-and-authorization.md)) and services, both via SMTP submission. No service embeds a provider SDK |
| Retries | delivery is a Temporal activity where it must be tracked, and the outbox where fire-and-forget is honest ([ADR-0302](0302-temporal.md)) |
| Local and non-prod | **Mailpit** as a sink. Non-production never delivers to a real recipient |
| Delivery observability | every send is a Temporal activity or an outbox row, so a submission failure is a failed activity with a retry history. Post-submission rejections arrive as DMARC aggregate reports, and the agent's logs are scraped like any other component's ([ADR-0500](0500-observability.md)). The signal that matters is the identity flow's completion rate: a verification mail that never lands shows up as a drop there, before a support ticket |

**Every sender speaks SMTP.** That single rule is what makes the exit below a configuration change: no service imports a provider client, so the relay target is a host, a port, and a credential.

### The exit is pre-built, not deferred

[ADR-0000](0000-platform-foundations.md) ranks this component first to concede. The seam therefore exists on day one rather than being designed under pressure.

| Field | Value |
| --- | --- |
| **Trigger** | a sustained delivery-failure or complaint rate against a major receiver that DMARC alignment and IP warmup do not resolve, or a listing on a mainstream blocklist that is not cleared within one business day |
| **Seam** | ✓ the relay host, port, and credential are values-file fields. Switching to a managed provider changes those three and the SPF record. No code changes |
| **Cost if adopted late** | a locked-out user population and support load while reputation is rebuilt. This is why the trigger is a measured rate rather than a judgement call |

## Consequences

### Positive

- Identity flows have a production delivery path owned end to end, and recipient addresses stay on controlled infrastructure.
- One Go binary and no datastore, against a floor that is the budget.
- The SMTP-only rule makes the sanctioned exit a values change, so the concession ADR-0000 anticipates is cheap when it is taken.

### Negative / Risks

- **Deliverability is a standing operational duty**, not a deploy. IP warmup, DMARC report review, and blocklist monitoring are recurring work for a small platform team. This is the cost [ADR-0000](0000-platform-foundations.md) names as irreducible, and it is accepted rather than mitigated.
- **A blocked port 25 makes self-hosting impossible on some providers.** Verify egress before provisioning, because the failure appears at first send rather than at deploy.
- **A reputation incident is slow to reverse.** The trigger above exists so the swap happens on a measurement rather than after a month of silent failures.
- **Mail is a third-party-observable side channel.** Content is minimal by construction: links and codes, never personal data ([ADR-0301](0301-data-lifecycle-privacy.md)).

## Rules

- Outbound mail leaves through a self-hosted maddy submission endpoint on a dedicated IP with a matching `PTR` record.
- The platform sends mail and does not receive it. No inbound listener, mailbox, or IMAP surface is deployed.
- Every sender speaks SMTP. No service embeds an email-provider SDK, so the relay target stays a configuration value.
- `SPF`, `DKIM`, and `DMARC` are committed per environment, and DMARC reaches `p=reject` before an environment is treated as production.
- Non-production environments deliver to a sink, never to a real recipient.
- Mail bodies carry links and codes, never personal data ([ADR-0301](0301-data-lifecycle-privacy.md)).
- Delivery failure is observable as a metric: a failed submission is a failed activity, and the identity flow's completion rate is the signal a dropped verification mail moves.
