# Incident Management

[`break-glass.md`](break-glass.md) covers access when the auth plane is down. This document covers process: what counts as an incident, who does what, and what is owed afterwards.

It is written for a platform where **nothing pages** ([ADR-0502](../adr/0502-alerting-and-on-call.md)). That is the constraint the severities below are drawn against, not an omission from them: a severity that assumes someone is woken is a severity this platform cannot honour, and [`../reference/detection-latency.md`](../reference/detection-latency.md) states what it can.

## Severity

Severity is about **user-visible impact**, never about how alarming the cause is. A saturated queue with nobody affected is not an incident; a checkout failing for one organisation is.

| Sev | Test | Response | Detection reality |
| --- | --- | --- | --- |
| **1** | Users cannot complete a core operation, or data integrity is at risk | drop other work; one responder, one comms owner | in hours, minutes. Out of hours, the next working day — the honest number, and the reason Sev 1 has an escalation trigger against it |
| **2** | A core operation is degraded, or a non-core one is failing. A workaround exists | same working day | as above |
| **3** | Contained fault with no user impact — a failing job, a stuck reconcile, a component in a bad state | next working day | as above |
| **4** | Something that will become an incident if untouched: a certificate near expiry, a volume near a threshold, a backup that failed once | this week | usually an alert, and never urgent yet |

**Severity is set at declaration and revised freely.** The first assessment is made with the least information anyone will have. Revising up is not an admission of error, and revising down is what closes an incident that a signal misrepresented.

## Roles

At this size the roles are hats, not people, and one person may wear two. What must not happen is one person wearing all three and dropping the third.

| Role | Owns | The failure it prevents |
| --- | --- | --- |
| **Responder** | the technical work: diagnosis, mitigation, the fix | — |
| **Comms** | who is told, and when. Anyone affected, anyone who might ask, and the record as it develops | the incident that is resolved and nobody outside it knows |
| **Scribe** | the timeline: what was observed, what was tried, what changed, with times | reconstructing from memory afterwards, which is where the useful detail dies |

For Sev 3 and 4, one person holds all three and the scribe's work is a few lines in the issue.

## The sequence

1. **Declare.** Open an issue in the forge ([ADR-0102](../adr/0102-source-control-and-ci.md)) with the severity and one sentence of user-visible impact. Declaring early and standing it down is cheap; the reverse is not.
2. **Mitigate before diagnosing.** Restoring service and understanding the cause are different jobs and the first outranks the second. A rollback is a mitigation ([ADR-0201](../adr/0201-gitops.md)); so is disabling a feature or shedding load.
3. **Diagnose from the funnel.** The three dashboard levels answer in order — what is wrong, which service, what is it doing ([ADR-0501](../adr/0501-operator-uis-and-dashboards.md)). [`../operational-surface.md`](../operational-surface.md)'s *What stops working* column reads down: if a component is the suspect, that column says what should already be broken, which confirms or eliminates it faster than a dashboard does.
4. **Record as you go.** Times, observations, and actions in the issue. The scribe writes what the responder says out loud.
5. **Resolve**, then state resolution to everyone who was told about the incident.
6. **Review**, per the threshold below.

## Postmortems

**Every Sev 1, every Sev 2 that recurs, and any incident someone asks for a review of.** Sev 3 and 4 close with their issue.

A postmortem is blameless in the operational sense: the subject is the system that let a correct-looking action have that effect, because a person's judgement in the moment is not a variable this platform can change and the system is.

| Section | Contains |
| --- | --- |
| Impact | who was affected, in what way, for how long |
| Timeline | first occurrence, first detection, mitigation, resolution. The gap between the first two is the detection latency this platform delivered |
| Cause | the conditions that made the failure possible, not the last change before it |
| What worked | the controls that caught it, or bounded it. These are as informative as the ones that did not |
| Actions | each with an owner and a place to live — a rule in an ADR, a check in CI, an alert, or a runbook entry |

**An action item without a home is not an action item.** The homes are: a rule in the owning ADR, a check in `mise run lint`, an alert rule, a row in [`../reference/deferral-register.md`](../reference/deferral-register.md), or a runbook under `docs/ops/`. An action that lives only in the postmortem is a postmortem nobody rereads.

**Two incident counts feed decisions already recorded.** Three distinct incidents in one quarter where a reported bug could not be reproduced from traces plus RUM logs is [ADR-0700](../adr/0700-analytics.md)'s session-replay trigger, and a second non-determinism failure in production is [ADR-0302](../adr/0302-temporal.md)'s versioning trigger. Both are counted from these records, so an incident that is never written down is a trigger that never fires.

## What this platform does not have

Stated plainly, because a process document that implies capabilities it lacks is worse than none.

- **No rota and no pager.** Out-of-hours detection is next working day, for every severity ([ADR-0502](../adr/0502-alerting-and-on-call.md)).
- **No incident-management tooling.** The forge's issues are the record, and there is no second place work is tracked ([ADR-0503](../adr/0503-error-tracking.md)).
- **No status page.** Comms reaches known affected parties directly. A public status page is a product decision a project makes for itself.
