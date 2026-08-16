# Working behind an HTTP proxy

Proxy configuration is a property of **your machine**, not of this template. The repo carries no proxy values and no proxy logic, and the `cluster:*` tasks work unchanged once the three one-time steps below are done. On a clean network, none of this applies.

The steps assume a **loopback** proxy on your host, for example `http://127.0.0.1:8118`. Each step is read from a different place — the host, a build container, the cluster node — so the address differs per step. That is the only subtlety. A **routable** proxy address works from everywhere: use it verbatim in every step and ignore the per-step address notes.

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
  "echo '$reg_ip registry.localhost k3d-registry.localhost' >> /etc/hosts"
kubectl --context kind-${CLUSTER:-platform} -n kube-system rollout restart deploy/coredns
```

The restart is the durable fix; the stamp survives only until the node is recreated.

## Step 3 — Export the proxy before the first cluster bring-up

Export it in the shell you bring a cluster up from, written as **your host** sees it — a loopback proxy stays `127.0.0.1`. kind cannot inject the proxy into the cluster node (the k3s-era create-time flags are gone), so a proxied machine preloads images on the host instead:

```sh
export HTTPS_PROXY=http://127.0.0.1:8118
CLUSTER_PRELOAD=1 mise run cluster:full
```

`cluster:preload` host-pulls the platform images — through the Docker daemon proxy, which handles large TLS blobs fine — and `kind load`s them into the node, so the node's containerd never has to reach the proxy. Anything that still wedges later (a chart bump pulling an image preload has not seen) is rescued by `mise run cluster:unwedge`, the same host-pull + load keyed to the stuck pods.

One more interaction, found the hard way: the Docker daemon's own proxy environment (step 1) IS inherited by the kind node container. Docker's embedded DNS answers a node with its IPv6 ULA (`fc00::/7`) address first, and if the daemon's `NO_PROXY` does not include that range, the node's own kubectl/kubelet dial the API server THROUGH the proxy and fail with EOF — which aborts `kind create` itself before the cluster is usable. Include `fc00::/7` in the daemon's `NO_PROXY` (the example in step 1 shows the range).

Everything below reads the inner loop's node ([ADR-0600](../adr/0600-local-development-loop.md)). The full tier is provisioned from a machine config, which carries its own proxy and registry-mirror settings, so its node name and its recovery path differ.

## Stalled image pulls

Even with the node proxied, some egress proxies time out or truncate large image layers on containerd's single-stream pull, leaving pods in `ImagePullBackOff`. Cilium, Argo CD, and the OpenFGA seed image are the usual victims.

```sh
mise run cluster:unwedge          # host-pull what is stuck, import it, restart the waiting pods
watch -n15 mise run cluster:unwedge   # or run it on a loop while cluster:full converges
```

Docker resumes and retries reliably where containerd does not, which is why the recovery routes through the host. Clean networks never need this.

## Restricted registries

On a network whose registry blocks **digest** pulls, so that only tags resolve, pre-pull the platform images by tag and `kind load docker-image` them. The upstream charts pin images by digest ([ADR-0104](../adr/0104-supply-chain-security.md)), which is what fails.
