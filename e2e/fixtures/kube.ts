// Minimal kubectl port-forward helper for the bootstrap. The Kratos admin API and
// SpiceDB are cluster-internal (never exposed at the edge); the e2e setup reaches
// them through a short-lived port-forward. kubectl/zed are on PATH via mise.
import { type ChildProcess, spawn } from "node:child_process";
import net from "node:net";

export type PortForward = { stop: () => void };

function probe(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const sock = net.connect({ host: "127.0.0.1", port }, () => {
      sock.destroy();
      resolve(true);
    });
    sock.on("error", () => resolve(false));
    sock.setTimeout(1000, () => {
      sock.destroy();
      resolve(false);
    });
  });
}

async function waitForPort(port: number, timeoutMs = 20_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await probe(port)) {
      return;
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`port-forward to 127.0.0.1:${port} did not come up within ${timeoutMs}ms`);
}

// portForward starts `kubectl -n <ns> port-forward svc/<svc> <local>:<remote>` and
// resolves once the local port accepts connections. Call stop() to tear it down.
export async function portForward(
  svc: string,
  local: number,
  remote: number,
  ns = "platform",
): Promise<PortForward> {
  const proc: ChildProcess = spawn(
    "kubectl",
    ["-n", ns, "port-forward", `svc/${svc}`, `${local}:${remote}`],
    { stdio: "ignore" },
  );
  proc.on("error", (err) => {
    throw new Error(`failed to spawn kubectl port-forward for ${svc}: ${err.message}`);
  });
  await waitForPort(local);
  return { stop: () => proc.kill() };
}
