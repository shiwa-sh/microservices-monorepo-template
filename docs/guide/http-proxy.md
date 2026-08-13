# Working behind an HTTP proxy

Proxy configuration is a property of **your machine**, not of this template. The repo carries no proxy values and no proxy logic, and the `cluster:*` tasks work unchanged once the three one-time steps below are done. On a clean network, none of this applies.

The steps assume a **loopback** proxy on your host, for example `http://127.0.0.1:8118`. Each step is read from a different place — the host, a build container, the cluster node — so the address differs per step. That is the only subtlety. A **routable** proxy address works from everywhere: use it verbatim in every step and ignore the per-step address notes.

## Step 1 — Proxy the Docker daemon, for image pulls

The daemon runs on your host, so a loopback proxy stays `127.0.0.1`. Create `/etc/systemd/system/docker.service.d/http-proxy.conf`:

```ini
[Service]
Environment="HTTP_PROXY=http://127.0.0.1:8118"
Environment="HTTPS_PROXY=http://127.0.0.1:8118"
Environment="NO_PROXY=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.svc,.svc.cluster.local,127.0.0.1,localhost,.localtest.me"
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

## Step 3 — Export the proxy before the first cluster bring-up

Export it in the shell you bring a cluster up from, written as **your host** sees it — a loopback proxy stays `127.0.0.1`. At create time the cluster task injects it onto the node, rewriting a loopback proxy to the node's host alias for you.

```sh
export HTTPS_PROXY=http://127.0.0.1:8118
mise run cluster:base
```

Everything below reads the inner loop's node ([ADR-0600](../adr/0600-local-development-loop.md)). The full tier is provisioned from a machine config, which carries its own proxy and registry-mirror settings, so its node name and its recovery path differ.

Verify the node received it — a loopback proxy should now read the host alias:

```sh
docker exec platform-control-plane env | grep -i proxy
```

The proxy is wired only at **create** time, so export it **before the first** bring-up. To rewire an existing cluster, recreate it with `mise run cluster:delete && mise run cluster:base`. If a node restart drops the host alias, re-add `<gateway-ip>` under the host alias in the node's `/etc/hosts`.

## Stalled image pulls

Even with the node proxied, some egress proxies time out or truncate large image layers on containerd's single-stream pull, leaving pods in `ImagePullBackOff`. Cilium, Argo CD, and the OpenFGA seed image are the usual victims.

```sh
mise run cluster:unwedge          # host-pull what is stuck, import it, restart the waiting pods
watch -n15 mise run cluster:unwedge   # or run it on a loop while cluster:full converges
```

Docker resumes and retries reliably where containerd does not, which is why the recovery routes through the host. Clean networks never need this.

## Restricted registries

On a network whose registry blocks **digest** pulls, so that only tags resolve, pre-pull the platform images by tag and `kind load docker-image` them. The upstream charts pin images by digest ([ADR-0104](../adr/0104-supply-chain-security.md)), which is what fails.
