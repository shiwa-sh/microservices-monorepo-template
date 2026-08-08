# Break-glass & ops recovery

How an operator reaches the debugging surfaces (Grafana, Hubble UI, Argo CD, the admin console) when the auth plane that normally gates them is itself down. Companion to [ADR-0306](../adr/0306-trust-tiers-and-urls.md) and [ADR-0304](../adr/0304-identity-and-authorization.md).

## Principle: no shared fate in the recovery path

The tools and credentials used to recover a system must not depend on that system. A recovery path that shares fate with the failed plane is a circular dependency: it passes every drill and fails the one real outage. Every mechanism below gives the ops tier an **independent trust root**.

Two moves, both taken here:

- **Logical decoupling** — same stack, fragile link removed. The ops tier's coarse gate is an `operator` **claim + AAL2**, not an OpenFGA `Checker` call ([ADR-0304](../adr/0304-identity-and-authorization.md)), so an OpenFGA outage does not lock operators out. Cheap, but Kratos/Oathkeeper are still in the path.
- **Physical decoupling (break-glass)** — a separate path with its own credentials that bypasses the plane entirely. This is what saves you when auth is fully down.

## The ladder for this stack

Right-sized for a small platform team — no separate operator IdP or PKI, which would re-introduce the two-auth-systems cost [ADR-0304](../adr/0304-identity-and-authorization.md) avoids.

1. **Everyday:** the SSO ops gate — Oathkeeper `operator` claim + AAL2 at `*.ops.<host>` ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)).
2. **Reduce the need for break-glass:** the coarse-gate-on-claim decoupling above. Only a full Kratos or Oathkeeper outage locks operators out; an OpenFGA outage does not.
3. **True break-glass (auth fully down): `kubectl port-forward` with an independently-obtained kubeconfig.** The kubeconfig authenticates to the API server via client cert/token — a trust root independent of Kratos and OpenFGA. It reaches any tool directly, bypassing Traefik, Oathkeeper, and OpenFGA:

   ```sh
   kubectl -n platform port-forward svc/grafana 3000:80       # then http://localhost:3000
   kubectl -n kube-system port-forward svc/hubble-ui 8080:80
   kubectl -n argocd   port-forward svc/argocd-server 8081:80
   ```

   This procedure is already printed by the `scripts/cluster-full.sh` banner as the diagnose path; it is the sanctioned break-glass.

## Requirements on the break-glass path

- **Pre-provisioned.** The kubeconfig must be obtainable *before* an outage and **must not be gated behind the product SSO** — otherwise it shares fate with the plane it recovers.
- **Fail secure, not fail open.** The ops gate never falls open when auth is unreachable. Recovery is a separate strong path, never a relaxed gate.
- **Loud and audited.** Break-glass use is logged and reviewed after the fact.
- **Tested.** Rehearse it (game-day / DiRT) alongside the DR drill ([ADR-0200](../adr/0200-cluster-topology.md)); an untested break-glass does not work when it is needed.

## Optional hardening

Seal Grafana/Argo **local-admin** credentials (independent of SSO) in a SOPS secret ([ADR-0202](../adr/0202-secrets.md)) as a secondary break-glass, disabled in normal operation. Not required while the kubeconfig path above is the sanctioned route.
