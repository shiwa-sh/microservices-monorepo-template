# ADR-0307: Outbound Email

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0200](0200-cluster-topology.md), [ADR-0202](0202-secrets.md), [ADR-0301](0301-data-lifecycle-privacy.md), [ADR-0302](0302-temporal.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0500](0500-observability.md)

## Context

Identity depends on mail. [ADR-0304](0304-identity-and-authorization.md)'s Kratos flows send address verification and account recovery, and neither has a fallback: an undelivered recovery mail is a locked-out user. Product notifications ride the same path, dispatched through the outbox or a workflow ([ADR-0302](0302-temporal.md)).

Outbound email is the component [ADR-0000](0000-platform-foundations.md) ranks **first** to concede when moving down axis B, because its principal cost is not operational but **reputational**. Standing the server up is a small task; being accepted is not, because acceptance is decided by receivers from the sending IP's history, the domain's authentication records, and complaint rates. Engineering effort cannot retire that cost, only accumulate it.

Two constraints shape every option. Many hosting providers block outbound port 25 by default. Residential and cloud IP ranges carry pre-existing reputation the platform inherits rather than earns.

**Scope: outbound only.** The platform sends mail. It does not receive it, host mailboxes, or run IMAP, so the inbound half of every mail suite is out of scope.

**Human mailboxes are a separate system, bought or self-hosted.** A team wanting `admin@example.com` as a real inbox needs correspondence — storage, IMAP, webmail, spam filtering, retention — which shares a domain with transactional sending and little else. The two coexist rather than merge: `MX` points at the mailbox system, `SPF` lists both senders, and each signs with its own `DKIM` selector. Needing mailboxes therefore never revises this decision; which system serves them is a separate choice, sketched under [If mailboxes are self-hosted](#if-mailboxes-are-self-hosted).

## Decision drivers

1. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). Mail leaves infrastructure we control, as the product's own domain.
2. **The reputation asset is the domain and the IP**, not the software. Novelty is priced accordingly (principle 4), because swapping the agent forfeits nothing that was earned.
3. **A reputation is shared with whoever sends beside it.** Driver 2's asset is spent as easily as it is earned, and any sender on the same IP and signing domain spends it. Account recovery has no fallback; human correspondence does. The two therefore never share a sender.
4. **Thinnest viable platform** (principle 2). A submission agent, not a mail suite.
5. **Switching sender costs a configuration change.** [ADR-0000](0000-platform-foundations.md) concedes this component first, so whatever is chosen must not be reachable from application code.
6. **A silently dropped mail is visible as a metric**, not as a support ticket.

## Considered options

| Option | Sends as our domain | Component weight | What it costs | Verdict |
| --- | --- | --- | --- | --- |
| **maddy, submission and DKIM only** | yes | **one Go binary**, no datastore | we own IP warmup, blocklist monitoring, and DMARC reporting | **Chosen.** Holds principle 3 at the lowest component count, and the software is cheap to swap because the reputation lives elsewhere |
| Postfix | yes | one daemon, plus its own spool | the same reputation work, plus a configuration language of its own | The boring choice, and principle 4 says be boring where exit costs months. Exit cost here is a config rewrite, not a migration — the reputation is unaffected by which agent sends. Rejected on operational legibility, not on maturity |
| OpenSMTPD | yes | one daemon | the same reputation work | The direct answer to the legibility complaint above: its configuration language is the smallest of any full MTA. It loses narrowly on DKIM, which is a filter to wire up rather than a built-in, so the thing this ADR most needs is the one part not in the box |
| A relay-only client — msmtp, nullmailer, or Postfix as a null client | **no** — it relays through some other sender | none worth counting | none of the reputation work, because it does none of the sending | Not an alternative but a shape: it presumes the managed row below. Worth naming because "self-host the SMTP endpoint" and "own the deliverability" are separable, and only the second is expensive |
| Stalwart | yes | one binary, broader scope | an inbound, JMAP, and mailbox surface we do not use | Capable and young. Rejected for carrying the inbound half this ADR scopes out — which is precisely why it is the preferred option [if mailboxes are self-hosted](#if-mailboxes-are-self-hosted) |
| Mailu / Mailcow | yes | a suite — several containers, a datastore, webmail, antispam | a full mail platform | Rejected by principle 2. These solve *running an email provider*, which is not the problem |
| poste.io | yes | a suite in one container — Postfix, Dovecot, Rspamd, webmail, admin UI | a full mail platform, inbound included | Rejected by principle 2, alongside Mailu and Mailcow. Packaging the suite as a single container lowers the operational count but not the surface: mailboxes, IMAP, and antispam are still deployed and still out of scope |
| Postal | yes | Ruby, MariaDB, RabbitMQ | a second datastore and a second message broker | Rejected by principles 2 and 5 |
| Managed transactional provider (Postmark, SES, Mailgun) | yes | none | deliverability becomes someone else's problem, and recipient metadata leaves our control | **The sanctioned exit**, ranked first on [ADR-0000](0000-platform-foundations.md)'s swap list. Not the default, because principle 3 is a constraint rather than a preference |
| Do nothing — the honest baseline | n/a | none | — | Account verification and recovery have no delivery path, so identity is unusable |

## Decision

| Concern | Decision |
| --- | --- |
| Agent | **maddy**, configured as a submission endpoint and DKIM signer. No inbound listener, no mailboxes |
| Egress | a **dedicated static IP** whose `PTR` resolves to `mail.example.com`, matching maddy's HELO name. Shared or dynamic egress is not used for mail |
| Authentication records | platform mail authenticates as `mail.example.com`: that subdomain carries maddy's `SPF` and its `DKIM` selector, leaving the organisation domain's records to whatever serves human mail. `DMARC` is published once at the organisation domain and governs the subdomain through `sp=`, so both senders report into one place. All are committed alongside the environment's other DNS ([ADR-0200](0200-cluster-topology.md)), starting at `p=none` with reporting and moving to `p=reject` once reports are clean |
| Signing key | the DKIM private key is SOPS-encrypted ([ADR-0202](0202-secrets.md)) |
| Senders | Kratos ([ADR-0304](0304-identity-and-authorization.md)) and services, both via SMTP submission. No service embeds a provider SDK |
| Retries | delivery is a Temporal activity where it must be tracked, and the outbox where fire-and-forget is honest ([ADR-0302](0302-temporal.md)) |
| Human mailboxes | not a platform concern, and never the same sender as platform mail. Whether bought or self-hosted, they serve the domain's `MX` from their own egress IP and `DKIM` selector, independent of maddy |
| Local and non-prod | **Mailpit** as a sink. Non-production never delivers to a real recipient |
| Delivery observability | every send is a Temporal activity or an outbox row, so a submission failure is a failed activity with a retry history. Post-submission rejections arrive as DMARC aggregate reports, and the agent's logs are scraped like any other component's ([ADR-0500](0500-observability.md)). The signal that matters is the identity flow's completion rate: a verification mail that never lands shows up as a drop there, before a support ticket |

**Every sender speaks SMTP.** That single rule is what makes the exit below a configuration change: no service imports a provider client, so the relay target is a host, a port, and a credential.

### Platform mail and human mail are never the same sender

Both may be self-hosted. They are still two senders, because merging them degrades the identity path in three ways:

- **Reputation.** One compromised staff mailbox sending spam for an afternoon, or a forwarded newsletter drawing complaints, degrades the IP that delivers password resets. Account recovery has no fallback; staff mail does.
- **Availability.** A suite upgrade, a full mailbox store, or a webmail CVE becomes an identity incident once the suite also carries platform mail.
- **Exit.** [ADR-0000](0000-platform-foundations.md) ranks platform sending first to concede. Coupled to a mailbox system, that exit means unpicking a live correspondence system instead of changing three values.

The separation only counts if it is physical:

| Axis | Platform mail | Human mail |
| --- | --- | --- |
| Sender | maddy | the mailbox system |
| Egress IP | dedicated, with its own `PTR` | its own, never shared with platform mail |
| Envelope domain | `mail.example.com` | `example.com` |
| DKIM selector | its own | its own |
| `MX` | none — it sends only | points here |

Sharing an egress IP or a signing domain forfeits the reputation argument entirely. Where a deployment cannot give platform mail its own IP, running maddy beside a suite buys nothing: let the suite send, and record the coupling as accepted rather than discovering it later.

### If mailboxes are self-hosted

Out of scope as a platform component, but the choice interacts with [ADR-0200](0200-cluster-topology.md) enough to be worth ranking. The verdicts in the options table above reject these as *platform senders*; this ranks them at the different job of hosting correspondence.

| Suite | Shape | Runs in-cluster | Verdict |
| --- | --- | --- | --- |
| **Stalwart** | one Rust binary — SMTP, IMAP, JMAP, spam filtering, admin UI, no external datastore required | yes, one deployment and a volume | **Preferred.** The only candidate whose operational shape matches the rest of the platform. Young against the Dovecot lineage, which is the price |
| **Mailu** | a container suite — Postfix, Dovecot, Rspamd, Roundcube, admin — with an official Helm chart | yes | The conservative pick: proven components, deployable in-cluster. Costs several workloads and a datastore where Stalwart costs one |
| **Mailcow** | ~15 containers, Docker Compose only; Kubernetes is explicitly unsupported upstream | no | The best admin experience and deepest community of the four, but it wants a dedicated VM. Choose it only by accepting that machine as infrastructure outside the cluster |
| **poste.io** | one container, proprietary core with the useful features behind a paid tier | yes | Rejected. Self-hosting a closed-source blob inverts the reason for self-hosting |

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
- **`PTR` delegation is a provider capability, not a given.** The dedicated egress IP needs reverse DNS resolving to maddy's HELO name, and providers vary: some do not offer it, some only by support request, and some not at all for load-balancer addresses. Receivers treat a missing or mismatched `PTR` as a strong negative signal, so this fails the same way blocked egress does — at first send. It is confirmed at provider selection ([ADR-0200](0200-cluster-topology.md)).
- **Two senders means two reputations to keep.** Where mailboxes are self-hosted, the separation this ADR mandates costs a second static IP, a second warmup, and a second set of blocklist checks. Coupling them would be worse — it puts account recovery behind staff mail's complaint rate — but the cost is real and lands on the same small team.
- **A reputation incident is slow to reverse.** The trigger above exists so the swap happens on a measurement rather than after a month of silent failures.
- **Mail is a third-party-observable side channel.** Content is minimal by construction: links and codes, never personal data ([ADR-0301](0301-data-lifecycle-privacy.md)).

## Rules

- Outbound mail leaves through a self-hosted maddy submission endpoint on a dedicated IP with a matching `PTR` record.
- The platform sends mail and does not receive it. No inbound listener, mailbox, or IMAP surface is deployed *as part of the platform*; a mailbox system, if one exists, is separate infrastructure.
- Every sender speaks SMTP. No service embeds an email-provider SDK, so the relay target stays a configuration value.
- Platform mail and human mailboxes are never the same sender. They use separate egress IPs and separate `DKIM` selectors whether the mailbox system is bought or self-hosted, and platform mail sends as a subdomain.
- `SPF`, `DKIM`, and `DMARC` are committed per environment, and DMARC reaches `p=reject` before an environment is treated as production.
- Non-production environments deliver to a sink, never to a real recipient.
- Mail bodies carry links and codes, never personal data ([ADR-0301](0301-data-lifecycle-privacy.md)).
- Delivery failure is observable as a metric: a failed submission is a failed activity, and the identity flow's completion rate is the signal a dropped verification mail moves.
