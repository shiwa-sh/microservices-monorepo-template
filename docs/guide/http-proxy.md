# Working behind an HTTP proxy

Proxy configuration is a property of **your machine**, not of this template. The repo carries no proxy values and no proxy logic, and the `cluster:*` tasks work unchanged once the three one-time steps below are done. On a clean network, none of this applies.

**Most of the image-pull steps below are now optional.** zot mirrors docker.io, quay.io, ghcr.io and registry.k8s.io as a pull-through cache ([ADR-0105](../adr/0105-image-registry.md)), so once it is up the cluster nodes fetch images from it rather than from the internet — one component with egress instead of every node. What still needs a proxy is the **build** path (base images, Go modules, bun) and non-image traffic such as ACME. Steps 1 and 2 are therefore about your host and your builds; step 3 matters only before the mirror exists.

The steps assume a **loopback** proxy on your host, for example `http://127.0.0.1:8118`. Each step is read from a different place — the host, a build container, the cluster node, a cluster pod — so the address differs per step. That is the only subtlety. A **routable** proxy address works from everywhere: use it verbatim in every step and ignore the per-step address notes.

## Start here

```sh
mise run proxy:setup              # apply everything that does not need root
mise run proxy:setup -- --check   # what is configured, what is missing, and the exact fix
```

It reads the live state rather than the intent — what the docker daemon reports, what is in your `~/.docker/config.json`, whether the cluster node can resolve, whether the repo-server carries the proxy — does the per-step address arithmetic for you, and names the step that is wrong. On a direct network it exits 0 and tells you none of this applies.

Only step 1 needs root, because it edits a systemd unit. `--fix` writes that file for you and prints the two `sudo` lines to paste; everything else it applies itself.

Read the rest of this page when you want to know *why* a step exists, or when the doctor reports something it cannot fix. The steps below are the reference; the command above is the path.

## Step 1 — Proxy the Docker daemon, for image pulls

The daemon runs on your host, so a loopback proxy stays `127.0.0.1`. Create `/etc/systemd/system/docker.service.d/http-proxy.conf`:

```ini
[Service]
Environment="HTTP_PROXY=http://127.0.0.1:8118"
Environment="HTTPS_PROXY=http://127.0.0.1:8118"
Environment="NO_PROXY=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7,.svc,.svc.cluster.local,127.0.0.1,localhost,.localtest.me"
```

Then `sudo systemctl daemon-reload && sudo systemctl restart docker`.

## Step 2 — Proxy Docker builds

Docker injects `~/.docker/config.json` → `proxies.default` into `docker build` RUN steps. There the reader is **inside a container**, where `127.0.0.1` would mean the container itself, so use the **docker-bridge gateway IP**. Find it with:

```sh
docker network inspect bridge -f '{{(index .IPAM.Config 0).Gateway}}'    # usually 172.17.0.1
```

```json
{
  "proxies": {
    "default": {
      "httpProxy": "http://172.17.0.1:8118",
      "httpsProxy": "http://172.17.0.1:8118",
      "noProxy": "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.svc,.svc.cluster.local,127.0.0.1,localhost,.localtest.me"
    }
  }
}
```

Keep the package and image registries **out** of `noProxy` so they route through the proxy. A loopback value here is the classic build failure: the package manager inside the build cannot see the proxy, goes direct, and the firewall blocks it.

## Step 2b — DNS for the daemon's embedded resolver

The Docker daemon caches the host's `/etc/resolv.conf` nameservers when it starts, and every container's embedded DNS forwards external names to that cached set. Change the host's resolver afterwards (a new gateway or an `nmcli` switch), and every container — the kind node included — silently loses external resolution: `docker exec <node> getent hosts github.com` returns nothing while the host resolves it fine. Docker's embedded DNS still answers container names (`registry.localhost`, the node's own hostname), so the failure is easy to misread as a cluster problem.

The fix is the same category as step 1: a one-time daemon restart so it re-reads the current resolver. Verify the embedded DNS forwards where you expect before blaming anything else:

```sh
docker exec <node> getent hosts github.com    # empty while the host resolves → stale upstream
sudo systemctl restart docker
```

Until the daemon is restarted, a running cluster can be unblocked per-node by pointing the node at a working resolver and stamping the registry container names into its `/etc/hosts` (docker does not rewrite an edited resolv.conf):

```sh
node=$(kubectl --context kind-${CLUSTER:-platform} config view --minify -o jsonpath='{.clusters[0].cluster.server}')
docker exec "${CLUSTER:-platform}-control-plane" sh -c \
  'printf "nameserver 1.1.1.1\n" > /etc/resolv.conf'
reg_ip=$(docker inspect registry.localhost \
  --format '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' | cut -d' ' -f1)
docker exec "${CLUSTER:-platform}-control-plane" sh -c \
  "echo '$reg_ip registry.localhost' >> /etc/hosts"
kubectl --context kind-${CLUSTER:-platform} -n kube-system rollout restart deploy/coredns
```

The restart is the durable fix; the stamp survives only until the node is recreated.

## Step 3 — The environment `cluster:up` runs kind in

`mise run proxy:setup` writes `infra/local/proxy.local.env`, and `scripts/lib/cluster.sh` loads it, so every cluster command runs with this machine's egress and no cluster script names a proxy. The file is machine-local and gitignored. Nothing has to be exported by hand:

```sh
mise run cluster:up -- full
```

**It exists because kind reads the proxy variables from its own environment.** The node's kubeadm dials the API server *by name* — `https://<cluster>-control-plane:6443` — and no CIDR in a `NO_PROXY` list exempts a hostname, so that call goes to the proxy and dies. kind is the only thing that can prevent it: when it sees the variables it takes them over and appends what the node needs, the node's own name included.

```text
no_proxy=fc00:f853:ccd:e793::/64,172.19.0.0/16,localhost,127.0.0.1,10.96.0.0/16,
         10.244.0.0/16,<cluster>-control-plane,.svc,.svc.cluster,.svc.cluster.local
```

Without them, the node still receives a proxy — the docker CLI injects `proxies.default` from `~/.docker/config.json` (step 2) into every container it creates — but that value carries no node name, and `kind create` aborts partway through:

```text
✗ Starting control-plane 🕹️
ERROR: failed to create cluster: failed to remove control plane taint: ...
Get "https://<cluster>-control-plane:6443/api?timeout=32s": EOF
```

The address written here is the **host's**, because everything that reads the file runs on the host. Images are unaffected either way: `cluster:up` fills the local zot before it creates the cluster and the nodes pull only from there, so a proxy that cannot reach an upstream fails while warming, naming the image, rather than as an `ImagePullBackOff` twenty minutes into a sync ([ADR-0105](../adr/0105-image-registry.md)).

## Step 4 — Proxy Argo CD's repo-server, for git and chart repositories

`mise run proxy:setup` writes this one to `infra/local/proxy.local.yaml`, and every `cluster:up -- full` applies it when installing Argo CD. That is why the file exists rather than a `kubectl patch`: the repo-server does not exist until a full tier has been brought up, so a fix that only patches a running Deployment fixes the *second* bring-up and hangs the first. The file is machine-local and gitignored — the address is your host's docker gateway.

Steps 1–3 get **images** into the cluster. They do nothing for a workload that opens its own connection to the internet, and Argo CD's repo-server is exactly that: to render a chart with `dependencies:`, it runs `helm repo add` against each upstream repository from inside the pod. Preloaded images do not help, because nothing was pulling an image.

The failure is quiet and easy to misattribute. The app reports `Unknown` with a `ComparisonError` and its already-running workloads stay `Healthy`. The repo-server clones the git remote as well, so on a network that blocks it the root app itself never renders and the tier stops before any application appears. If the app is a wave gate, the root app-of-apps stalls behind it and the whole tier looks stuck for a reason that is nowhere near the tier.

```text
ComparisonError  Failed to load target state: ... error building helm chart dependencies:
                 failed to add helm repository https://k8s.ory.sh/helm/charts: ...
                 failed running helm: `helm repo add ...` failed timeout after 1m30s
```

The tell that this is the proxy and not the chart: the **same command succeeds on your host**, because your shell has the proxy exported and the pod does not. Do not conclude the repository is down or the URL has moved. Compare like for like — run it in the pod:

```sh
pod=$(kubectl -n argocd get pod -l app.kubernetes.io/name=argocd-repo-server \
  -o jsonpath='{.items[0].metadata.name}')
kubectl -n argocd exec "$pod" -- helm repo add probe <repo-url>   # hangs, then times out
```

The reader here is **a pod**, so a loopback proxy needs the **kind network gateway**:

```sh
docker network inspect kind -f '{{range .IPAM.Config}}{{.Gateway}} {{end}}'   # take the IPv4
```

Set it through the chart's `repoServer.env`. `infra/helm/platform/argocd` is a wrapper around the upstream `argo-cd` chart, so the key is nested under the subchart alias — a bare `repoServer.env` is silently ignored and renders no env at all. This is machine config, so keep it out of the committed values file: pass it at upgrade time, or from an untracked overlay. Quote every `--set`, or the shell globs the `[0]`.

```sh
helm upgrade argocd infra/helm/platform/argocd -n argocd --timeout 8m \
  --set 'argo-cd.repoServer.env[0].name=HTTPS_PROXY' \
  --set 'argo-cd.repoServer.env[0].value=http://172.19.0.1:8118' \
  --set 'argo-cd.repoServer.env[1].name=HTTP_PROXY' \
  --set 'argo-cd.repoServer.env[1].value=http://172.19.0.1:8118' \
  --set 'argo-cd.repoServer.env[2].name=NO_PROXY' \
  --set-string 'argo-cd.repoServer.env[2].value=localhost\,127.0.0.1\,.svc\,.svc.cluster.local\,.cluster.local\,10.0.0.0/8\,.localtest.me'
```

Confirm helm rendered it, rather than reading the live Deployment — a hand-run `kubectl set env` survives the 3-way merge and will show you your own patch either way:

```sh
helm get manifest argocd -n argocd | grep -A1 'name: HTTPS_PROXY'
```

`NO_PROXY` must keep cluster-internal traffic direct; without it the repo-server dials the API server and its own services through the proxy. Verify with the same in-pod `helm repo add` — it should return in about a second — and then force Argo to discard what it cached while the network was broken:

```sh
argocd app get <app> --core --hard-refresh
```

A plain `--refresh` is not enough. The generation error is cached with the manifests, so the app keeps reporting the old `ComparisonError` verbatim long after the fix landed, which reads as the fix having failed.

### Do not reach for CoreDNS

The symptom invites a DNS fix, because a chart repo on GitHub Pages resolves to four anycast addresses and a network that blackholes some of them fails about half the time. Pinning the good addresses in a CoreDNS `hosts` block, or suppressing AAAA with a `template ANY AAAA { rcode NOERROR }` stanza, both appear to help and neither is the fix.

**Once the proxy is set, the pod never dials those addresses itself** — the proxy resolves and connects on its behalf, so what the cluster resolves the name to stops mattering. Verified by removing both stanzas: the repo-server then resolves the chart host to IPv6 only, exactly the condition the AAAA stanza existed to prevent, and `helm repo add` still returns in 1–3s.

Keep CoreDNS carrying only what the platform put there — for the inner loop, the `dev.localtest.me` `rewrite stop` blocks from `scripts/lib/cluster.sh`. Live Corefile edits are invisible to Argo, survive no rebuild, and outlive the problem they were added for. If you have already made them, remove them and re-verify rather than leaving them in place: a workaround nobody can attribute is worse than the fault it hid.

One caveat when you do. Rolling CoreDNS kills in-flight lookups, so unrelated apps briefly go `Unknown` with `failed to list refs: ... EOF` against GitHub. That is the rollout, not the removal — confirm with `git ls-remote` from inside the repo-server, then clear it with `--hard-refresh` for the same caching reason as above.

Everything below reads a local kind node. Both local tiers are kind ([ADR-0600](../adr/0600-local-development-loop.md)), so the node name is the only thing that differs between them.

## Talos nodes — the machine config, not the shell

This section is about **deployed** environments ([ADR-0200](../adr/0200-cluster-topology.md)); no local tier applies a machine config.

The steps above configure a kind node, which inherits a good deal from the host docker daemon. A Talos node inherits nothing: there is no shell, no daemon configuration, and no environment to export into. The proxy is part of the machine config or it does not exist.

```yaml
machine:
  env:
    http_proxy: http://10.5.0.1:8118
    https_proxy: http://10.5.0.1:8118
    no_proxy: localhost,127.0.0.1,10.96.0.0/12,10.244.0.0/16,.svc,.cluster.local
```

Two things catch people, and both were measured rather than guessed:

**`127.0.0.1` is the node.** A proxy on your host's loopback is not reachable from inside a node by that address; use the address the node can route to — a routable address for real hardware.

**`no_proxy` must carry the pod and service CIDRs.** Without them the kubelet, the API server and every pod-to-pod call go out through the proxy, which fails differently and later than a missing proxy does.

**The failure never mentions the proxy.** Without this a node reports `403 Forbidden` fetching `registry.k8s.io/etcd`, etcd never leaves `Preparing`, and the bootstrap times out on "waiting for etcd to be healthy" — which reads as a broken cluster rather than a blocked network. Measured, and the most expensive way to learn it.

## Stalled image pulls

A slow proxy makes a cold registry slow, and zot copies a whole image before it answers the request that triggered it. Left to the cluster that is fatal: every pod asks at once, zot saturates, and each pull exceeds containerd's deadline. Warming ahead of the cluster is what avoids it, and it is unconditional — `cluster:up` does it on every network.

If a warm does fail, it names the images it could not cache and stops before creating a cluster. Re-run it: images already cached are skipped in microseconds, so a retry only pays for what is still missing.

## Restricted registries

On a network whose registry blocks **digest** pulls, so that only tags resolve, pre-pull the platform images by tag and `kind load docker-image` them. The upstream charts pin images by digest ([ADR-0104](../adr/0104-supply-chain-security.md)), which is what fails.

`kind load` works on both tiers, so this recovery is the same wherever it is needed. Pass `--name` to reach the full tier's cluster.
