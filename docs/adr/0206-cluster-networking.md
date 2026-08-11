# ADR-0206: Cluster Networking

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0200](0200-cluster-topology.md), [ADR-0203](0203-policy-enforcement.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0500](0500-observability.md), [ADR-0501](0501-operator-uis-and-dashboards.md)
- **Decides:** Cilium is the CNI from bootstrap, with WireGuard encryption and default-deny in both directions, and no service mesh runs above it.

## Context

[ADR-0200](0200-cluster-topology.md) puts Talos on plain instances and removes the host's attack surface. Neither that nor Pod Security Admission constrains which pod may open a connection to which, and that question is settled at bootstrap: **CNI cannot be hot-swapped on a live cluster.**

The answer is load-bearing beyond networking. Internal calls carry forwarded identity headers and no token ([ADR-0305](0305-edge-auth-and-traffic-policy.md)), so what guarantees that only sanctioned callers reach a service's port is the network policy rather than the service.

This ADR decides the CNI, the postures on from day one, and whether a service mesh runs above it. Traffic reaching the cluster from outside is [ADR-0305](0305-edge-auth-and-traffic-policy.md); which layer enforces which class of invariant is [ADR-0203](0203-policy-enforcement.md).

## Decision drivers

1. **The pod network is not a trust boundary by default.** Whatever provides the CNI decides whether a compromised pod can reach Postgres.
2. **The header-trust concession needs a network guarantee.** [ADR-0305](0305-edge-auth-and-traffic-policy.md) accepts that a service trusts `X-User-Id` because default-deny bounds who can send it. Weakening the network weakens an accepted risk somewhere else.
3. **One primitive per concern** ([ADR-0000](0000-platform-foundations.md), principle 5). Encryption, identity, L4 policy, and flow visibility are one component's job or four.
4. **Nothing on the hot path per service** ([ADR-0000](0000-platform-foundations.md)). A per-pod proxy is a cost multiplied by the service count.

## Considered options

### East-west security

**Neither layer adjacent to the CNI provides east-west security.** A service mesh sits above it, riding on a CNI; Talos's own networking sits below it, on the host. The day-one decision is therefore at the CNI layer, compared on security capability. Linkerd is the mesh column, as the lightest of them: a mesh that loses these rows in its best case loses them in every case.

**Talos's networking is node-scoped.** The [host firewall](https://docs.siderolabs.com/talos/latest/networking/host-firewall) governs the node's own ports — kubelet, etcd, the API server — and never sees pod-to-pod traffic. KubeSpan encrypts links between nodes, so it segments nothing and does not touch same-node pod traffic. Both harden the machine; driver 1 is the network between pods.

| Capability | flannel only | Calico, eBPF dataplane | flannel + KubeSpan + Calico | flannel + Linkerd | **Cilium + WireGuard** |
| --- | --- | --- | --- | --- | --- |
| Components to operate | 1 | 1 | **3** | 2 | **1** |
| L3/L4 default-deny segmentation | **none — flat network** | all pods | all pods | meshed app traffic only | all pods |
| Data-tier protection (Postgres, object storage, OpenFGA) | wide open | NetworkPolicy | NetworkPolicy | only if the data tier is meshed, which the Job-heavy bootstrap resists | NetworkPolicy |
| Cryptographic workload identity | — | — | — | mTLS certs, meshed only | label identity; SPIFFE optional later |
| Encryption in transit, east-west | **plaintext** | all pods, WireGuard | node to node only | meshed only | all pods, WireGuard |
| Services without kube-proxy | no | yes, in the eBPF dataplane | **no — policy-only mode is iptables** | no | yes |
| L7 authz by route and method | — | limited, and Envoy-based in the paid tier | — | fine-grained | coarse, via Envoy |
| Egress control, DNS/FQDN, metadata SSRF | — | CIDR egress; **FQDN in the paid tier** | CIDR egress | not Linkerd's concern | FQDN and L3 egress in the open distribution |
| Per-flow visibility | — | **denied-packet metrics; flow logs are paid** | — | mesh-only | Hubble, from the same datapath |
| Verdict | policy is accepted and never enforced | **runner-up**; both capabilities it paywalls are committed here | most components, fewest capabilities | mesh-scoped, and additive to a CNI rather than a substitute | **Chosen** *(reasoned)* |

**flannel ships no NetworkPolicy controller**, so the API server accepts a policy and nothing enforces it — worse than having no policy, because it grants false confidence.

**Calico is the runner-up.** Its open distribution matches segmentation, WireGuard encryption of all pod traffic, and kube-proxy replacement through eBPF. Both capabilities it paywalls are load-bearing here: FQDN egress, without which a policy degrades to hand-maintained CIDR lists per external dependency, and the per-flow surface [ADR-0501](0501-operator-uis-and-dashboards.md) commits as this ADR's audit trail. Its compensating iptables dataplane, which any engineer can inspect, is forfeited by the eBPF mode that drops kube-proxy.

**flannel + KubeSpan + Calico is the Talos-native path plus policy, and costs the most for the least.** Calico over flannel runs policy-only, so the dataplane is iptables and kube-proxy stays. KubeSpan requires the [discovery service](https://docs.siderolabs.com/talos/latest/configure-your-talos-cluster/system-configuration/discovery), whose hosted endpoint is a third-party runtime dependency for east-west encryption and whose self-hosted build carries a commercial licence, failing [ADR-0000](0000-platform-foundations.md) principle 3.

**Cilium wins on breadth.** The controls it adds map onto the highest-frequency cluster attacks — lateral movement to the data tier, and metadata-endpoint credential theft — and it covers CNI, Services, encryption, and observability as one component.

### Why no service mesh

**A mesh runs over a CNI, never instead of one.** The question is therefore not Cilium or a mesh but Cilium against Cilium plus a mesh: a second identity system and a second encrypted datapath on the same nodes, against one subtracted config flag.

**Sidecars are not what decides it.** Linkerd puts a proxy container on every pod's hot path, against driver 4. Istio's ambient mode removes that proxy — a per-node ztunnel carries L4 and mTLS, and waypoint proxies are added only where L7 policy is wanted — and the overlap below rules it out regardless.

| What a mesh adds | Who needs it | This platform |
| --- | --- | --- |
| L7 traffic management — weighted routing, mirroring, circuit breaking | percentage-based progressive delivery | no traffic-splitting delivery is committed. **This is the trigger that reopens the row**, and the one capability here Cilium lacks |
| L7 authorization imposed from outside the application | large polyglot estates, and code that cannot be changed | Oathkeeper at the edge, OpenFGA in-app ([ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0304](0304-identity-and-authorization.md)) |
| Per-request telemetry for uninstrumented workloads | applications without tracing | every service is instrumented ([ADR-0500](0500-observability.md)) |
| Per-workload certificate identity | any of the three conditions [ADR-0305](0305-edge-auth-and-traffic-policy.md) records against the header-trust concession it belongs to | label identity, with Cilium mutual auth and SPIFFE available later without sidecars |
| A multi-cluster fabric with locality failover | one service spanning clusters | one cluster per environment |

**FQDN egress stays a CNI job under either.** Istio's `ServiceEntry` with `REGISTRY_ONLY` matches on SNI and Host, which is policy for a cooperating workload rather than enforcement against a compromised one. Driver 1 asks for control in the datapath, and that is Cilium's in every column.

## Decision

### Cilium is the CNI from bootstrap

The [machine config](https://docs.siderolabs.com/kubernetes-guides/cni/deploying-cilium) sets `cluster.network.cni.name: none` and `cluster.proxy.disabled: true`, so Talos ships neither its default CNI nor kube-proxy, and Cilium provides both.

Cilium is delivered as an inline manifest in the machine config rather than installed afterwards. A node reports `NotReady` until a CNI runs and a cluster left without one reboots to retry, so shipping it with the bootstrap removes a timed race. Argo CD adopts the release afterwards for upgrades.

**CNI cannot be hot-swapped on a live cluster**, so the security posture is set at bootstrap rather than retrofitted.

### Three Talos properties are invariants, not preferences

| Constraint | Consequence |
| --- | --- |
| Workloads may not load kernel modules | `SYS_MODULE` is dropped from Cilium's default capability set |
| kube-proxy is absent | Cilium is given the API server host and port directly, because there is no in-cluster Service through which to discover it |
| **[KubeSpan](https://docs.siderolabs.com/talos/latest/networking/kubespan) is not enabled** | Talos's own WireGuard mesh intercepts inter-node traffic that Cilium's eBPF datapath expects on the primary interface, producing asymmetric routing and broken cross-node pod traffic. East-west encryption is Cilium's, once |

KubeSpan is the one worth stating twice, because enabling it looks like strictly more encryption and is a cross-node outage.

### Three postures, on from day one

| Posture | Mechanism |
| --- | --- |
| **Default-deny segmentation** | The `platform-baseline` CiliumNetworkPolicy sets `enableDefaultDeny` for ingress and egress across every platform pod. All allows are additive, and each service's chart declares which callers may reach it |
| **Encryption in transit** | `encryption.type: wireguard` encrypts all east-west pod traffic node to node. A config flag, not a component |
| **Egress control** | Default-deny egress means no pod reaches the internet unless a policy grants it. A clusterwide policy additionally denies `169.254.169.254/32`, so even a future broad egress grant cannot become an SSRF path to instance credentials |

Default-deny is load-bearing rather than defence in depth: it is what makes driver 2 hold.

### Traffic flow through the cluster

```text
Internet
  │
Provider Load Balancer  (L4, one stable public IP per env)
  │
Traefik  (TLS via cert-manager, L7 routing, rate limiting)
  ├── <host>/api/*                        ─▶ Oathkeeper ─▶ backend service   (ADR-0305)
  ├── <host>/(landing|panel|devportal)/*  ─▶ frontend pod                     (ADR-0400)
  ├── lowdefy.ops.<host>/                 ─▶ Oathkeeper ─▶ Lowdefy            (ADR-0401)
  ├── grafana.ops.<host>/                 ─▶ Oathkeeper ─▶ Grafana            (ADR-0501)
  └── hubble.ops.<host>/                  ─▶ Oathkeeper ─▶ Hubble UI          (ADR-0501)
```

**Traefik is the only ingress. Oathkeeper is an auth filter behind it, not a second gateway** ([ADR-0305](0305-edge-auth-and-traffic-policy.md)). There is no API-management gateway in the default stack.

**DNS.** One wildcard `A` record per environment points at the load-balancer IP, and cert-manager requests one wildcard certificate per environment through DNS-01. `external-dns` is not used, because the wildcard absorbs new services.

Two provider capabilities are therefore requirements rather than conveniences, and both are verified before an environment is provisioned:

- **A DNS provider API that cert-manager supports**, without which DNS-01 issuance has no path.
- **Reverse-DNS (`PTR`) delegation on the mail egress IP**, which [ADR-0307](0307-outbound-email.md) needs to match maddy's HELO name. Not every provider offers it, and where it is offered it is often manual, request-only, or unavailable for load-balancer addresses.

Both are cheap to confirm at provider selection and expensive to discover afterwards: a missing `PTR` surfaces as mail being rejected at first send, not as a failed deploy.

### Observability of the network

Hubble provides per-flow visibility and is the audit surface for these policies, through the CLI, the drop metrics in Grafana, and the auth-gated UI ([ADR-0501](0501-operator-uis-and-dashboards.md)). Application observability is Grafana's ([ADR-0500](0500-observability.md)).

Cilium covers CNI and mesh as one component: sidecarless eBPF gives transparent encryption, L7 policy, and per-flow observability without an injected proxy. Cilium mutual auth and SPIFFE can add certificate identity later without sidecars.

## Consequences

### Positive

- One component provides CNI, Services, east-west encryption, egress control, and flow visibility, which is driver 3 satisfied rather than argued.
- The network guarantee that [ADR-0305](0305-edge-auth-and-traffic-policy.md)'s header trust rests on is enforced in the datapath, so it constrains a compromised pod rather than a cooperating one.
- No per-pod proxy, so the per-service cost of the security posture is zero.
- A metadata-endpoint SSRF cannot be reached by any future egress grant, because the deny is clusterwide rather than per policy.

### Negative / Risks

- **Cilium is harder to debug than flannel** — eBPF programs, `cilium status`, the Hubble CLI. Mitigated by the chart being committed and Argo CD managing upgrades after bootstrap.
- **CNI is set at bootstrap and cannot be changed live.** This is why the posture is decided here rather than deferred, and it is why the exit is a cluster rebuild rather than a values change.
- **No L7 traffic management.** Weighted routing, mirroring, and circuit breaking are unavailable, and adopting them means adopting a mesh. The trigger is a committed progressive-delivery requirement.
- **KubeSpan is a foot-gun with a friendly name.** It reads as more encryption and breaks cross-node pod traffic, and nothing in the Talos configuration surface warns about the interaction.

## Rules

- CNI is Cilium from day one, delivered as an inline manifest in the machine config and adopted by Argo CD for upgrades. Talos ships neither its default CNI nor kube-proxy.
- KubeSpan is not enabled. East-west encryption is Cilium's WireGuard, and enabling both breaks cross-node pod traffic.
- Talos's host firewall governs the node's own ports and is never treated as pod-to-pod segmentation.
- `SYS_MODULE` is dropped from Cilium's capability set, and the API server host and port are set explicitly.
- Default-deny is enforced for ingress and egress across every platform pod; all allows are additive. `(enforced: CiliumNetworkPolicy)`
- WireGuard transparent encryption is on for all east-west pod traffic. Plaintext east-west is not shipped.
- A clusterwide policy denies `169.254.169.254/32`, so no egress grant can become a metadata-SSRF path. `(enforced: CiliumNetworkPolicy)`
- Cilium NetworkPolicy is the internal service-to-service trust boundary, and each service declares its allowed callers. `(CI: lint:service-contract)`
- No dedicated service mesh is deployed, sidecar or ambient. A mesh runs over the CNI rather than instead of it, so it is a second component re-providing encryption, identity, and L4 policy that Cilium already provides, and its L7 layer is already covered at the edge and in-app.
- One wildcard `A` record and one wildcard certificate per environment. `external-dns` is not used.
- An environment is provisioned only where the provider offers a cert-manager-supported DNS API and `PTR` delegation on the mail egress IP.
