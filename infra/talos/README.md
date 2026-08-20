# Node machine configuration

Every node in a provisioned environment runs Talos Linux, and this directory is
its configuration ([ADR-0200](../../docs/adr/0200-cluster-topology.md)). There is
no SSH, no shell, no package manager and no writable root filesystem. A node is
configured by applying a machine config document over its gRPC API, and
`talosctl` is the only other interface.

The practical consequence is that **node configuration drift is not mitigated
here, it is impossible**. The machine config is the node. Nothing converges it,
because there is no second path by which the two could diverge.

## What is here

| Path | What it is |
| --- | --- |
| `patches/common.yaml` | applied to every node whatever its role |
| `patches/controlplane.yaml` | the control-plane role, on top of `common.yaml` |
| `patches/worker.yaml` | the worker role, applied when [ADR-0200](../../docs/adr/0200-cluster-topology.md)'s scaling trigger fires |
| `inventory/<env>/nodes.yml` | the addresses of pre-provided nodes, and the endpoint clients dial |
| `schematic.yaml` | the [Image Factory](https://docs.siderolabs.com/talos/latest/learn-more/image-factory) recipe for the custom installer image |

## The two provisioning modes

ADR-0200 divides on provisioning and **only** on provisioning. Everything
downstream — machine configuration, Kubernetes, Cilium, Argo CD — is identical.

**A project that owns its infrastructure** runs Terraform (`infra/terraform/`),
which creates the instances and then applies these same documents through the
first-party `siderolabs/talos` provider. Machine-config apply and bootstrap become
a plan rather than a script.

**A project handed existing machines** skips Terraform entirely and applies these
documents to the nodes named in `inventory/<env>/nodes.yml`. Pre-provided means
**pre-provided Talos**: a fleet running something else is a reprovision, not a
configuration step.

## Applying

```sh
# Once per cluster: mint the machine secrets, which are the cluster's identity.
talosctl gen secrets -o secrets.yaml
sops --encrypt --in-place secrets.yaml   # ADR-0202 — never committed in the clear

talosctl gen config example https://kube.dev.example.com:6443 \
  --with-secrets secrets.yaml \
  --config-patch @patches/common.yaml \
  --config-patch-control-plane @patches/controlplane.yaml \
  --config-patch-worker @patches/worker.yaml

talosctl apply-config --insecure -n 203.0.113.11 -f controlplane.yaml   # per node
talosctl bootstrap -n 203.0.113.11                                      # ONCE, one node
```

`bootstrap` initialises etcd and runs on exactly one node, ever. Running it on a
second node makes a second cluster.

## Two things that will bite

**`cni: none` does not remove flannel from a running cluster.** Talos applies its
default manifests once at bootstrap and then leaves them alone. Patch an existing
cluster and the nodes come back `Ready` — on flannel — while the config says there
is no CNI. Installing Cilium on top leaves two CNIs contending for the dataplane,
which presents as intermittent pod-to-pod failure rather than as an error anyone
would connect to the cause. Delete `kube-flannel` and `kube-proxy` explicitly after
the patch. On a cluster bootstrapped with these documents from the start, neither
is ever created.

**Cilium's `k8sServiceHost` is KubePrism, not an API server.** It is `localhost:7445`,
a per-node load balancer over every control-plane endpoint, so the agent survives
losing whichever control plane it was pointed at. Reading `cilium-config` to check
this shows an empty value and looks like a misconfiguration: Cilium 1.19 carries
the address as `KUBERNETES_SERVICE_HOST`/`PORT` environment variables on the agent,
not in the ConfigMap.

## Planting the age key

A deployed environment's private age key is the root of trust for every secret the
platform decrypts ([ADR-0202](../../docs/adr/0202-secrets.md)), and the
sops-operator reads it from a Kubernetes Secret. There is no host filesystem to
place it on and no configuration-management agent to place it — so it rides in the
machine config itself, as an inline manifest:

```yaml
cluster:
  inlineManifests:
    - name: sops-age-key
      contents: |
        apiVersion: v1
        kind: Secret
        metadata:
          name: sops-age-key
          namespace: platform
        stringData:
          key.txt: AGE-SECRET-KEY-…
```

This is not committed in the clear. The machine config carrying it is SOPS-encrypted
like every other secret, which is the same protection the key would have anywhere
else — and it means cluster identity and the secret root of trust are recovered by
the same apply, rather than by a manual step someone has to remember during an
incident.

## Behind a proxy

A Talos node inherits nothing from anyone's shell. On a proxied network the node
cannot pull, and **the failure never mentions the proxy**: the node reports
`403 Forbidden` fetching `registry.k8s.io/etcd`, etcd never leaves `Preparing`, and
whatever is waiting on the cluster times out on "waiting for etcd to be healthy".
That reads as a broken cluster rather than as a blocked network, and it is the
single most expensive way to learn this.

The proxy goes in the machine config, and only there:

```yaml
machine:
  env:
    http_proxy: http://proxy.example.com:8118
    https_proxy: http://proxy.example.com:8118
    # Without the CIDRs the nodes proxy their own east-west traffic, which fails
    # differently and later.
    no_proxy: localhost,127.0.0.1,10.96.0.0/12,10.244.0.0/16,.svc,.cluster.local
```

Two things that catch people. `127.0.0.1` is the NODE inside a node — a proxy on
the operator's loopback has to be named by an address the node can route to. And
`no_proxy` must carry the pod and service CIDRs, or the API server, the kubelet and
every pod-to-pod call go out through the proxy.

## Secrets

The machine secrets — the cluster CA and `talosconfig` — are SOPS-encrypted in git
like every other secret ([ADR-0202](../../docs/adr/0202-secrets.md)). Cluster
identity is reproducible from git plus those secrets, which is what makes a
full-cluster loss recoverable: apply the configs, bootstrap, and let Argo CD
reconcile.
