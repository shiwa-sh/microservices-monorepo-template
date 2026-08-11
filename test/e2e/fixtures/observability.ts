// Observability query helpers for the end-to-end signal-correlation gauge
// (ADR-0011). The three backends are cluster-internal (only Grafana is edge-
// exposed), so the suite reaches Tempo/Loki/Prometheus through short-lived
// port-forwards (fixtures/kube.ts) and queries their native HTTP APIs directly.
//
// These functions are deliberately poll-friendly: OTLP export is batched and the
// stores ingest asynchronously, so callers wrap them in expect.poll / toPass.
import { expect } from "@playwright/test";
import { type PortForward, portForward } from "./kube";

// The three signal stores, each on its own local port while a test runs.
export const TEMPO_PORT = 13200;
export const LOKI_PORT = 13100;
export const PROM_PORT = 19090;

// A minimal port-forward set for the observability stores. Callers open it once in
// beforeAll and stop() it in afterAll.
export type ObsForwards = { stop: () => void };

export async function forwardObservability(): Promise<ObsForwards> {
  const pfs: PortForward[] = [
    await portForward("tempo", TEMPO_PORT, 3200),
    await portForward("loki", LOKI_PORT, 3100),
    await portForward("prometheus", PROM_PORT, 9090),
  ];
  return { stop: () => pfs.forEach((p) => p.stop()) };
}

// tempoTraceServices fetches a trace by id and returns the distinct
// resource.service.name values across its spans (empty if the trace is not yet
// queryable). This is the cross-service stitch assertion: a checkout trace must
// carry spans from orders, catalog and payment under one id.
export async function tempoTraceServices(traceId: string): Promise<string[]> {
  const res = await fetch(`http://127.0.0.1:${TEMPO_PORT}/api/traces/${traceId}`);
  if (!res.ok) {
    return [];
  }
  const body = (await res.json()) as {
    batches?: { resource?: { attributes?: { key: string; value?: { stringValue?: string } }[] } }[];
  };
  const svcs = new Set<string>();
  for (const b of body.batches ?? []) {
    for (const a of b.resource?.attributes ?? []) {
      if (a.key === "service.name" && a.value?.stringValue) {
        svcs.add(a.value.stringValue);
      }
    }
  }
  return [...svcs];
}

// tempoSearchByOrderId resolves the trace id for a checkout via the order.id span
// attribute the orders handler stamps (TraceQL `{ .order.id = "<id>" }`). Returns
// the first matching trace id, or "" if none is queryable yet.
export async function tempoSearchByOrderId(orderId: string): Promise<string> {
  const q = encodeURIComponent(`{ .order.id = "${orderId}" }`);
  const now = Math.floor(Date.now() / 1000);
  const res = await fetch(
    `http://127.0.0.1:${TEMPO_PORT}/api/search?q=${q}&limit=5&start=${now - 3600}&end=${now}`,
  );
  if (!res.ok) {
    return "";
  }
  const body = (await res.json()) as { traces?: { traceID: string }[] };
  return body.traces?.[0]?.traceID ?? "";
}

// lokiServicesForTrace queries Loki for log lines carrying a given trace_id in
// structured metadata (LogQL `| trace_id="<id>"`) and returns the distinct
// service_name labels that logged under it — proof logs↔traces correlate.
export async function lokiServicesForTrace(traceId: string, windowSec = 900): Promise<string[]> {
  const now = Math.floor(Date.now() / 1000);
  const q = encodeURIComponent(`{service_namespace="platform"} | trace_id="${traceId}"`);
  const res = await fetch(
    `http://127.0.0.1:${LOKI_PORT}/loki/api/v1/query_range?query=${q}` +
      `&start=${(now - windowSec) * 1_000_000_000}&end=${now * 1_000_000_000}&limit=200`,
  );
  if (!res.ok) {
    return [];
  }
  const body = (await res.json()) as {
    data?: { result?: { stream?: Record<string, string> }[] };
  };
  const svcs = new Set<string>();
  for (const s of body.data?.result ?? []) {
    const name = s.stream?.service_name;
    if (name) {
      svcs.add(name);
    }
  }
  return [...svcs];
}

// promSeriesCount runs an instant query for a metric and returns how many series
// matched. Prometheus escapes OTLP names to the classic underscore form, so callers
// pass e.g. "orders_checkouts_started_total" (the name is quoted into the selector,
// which is valid PromQL for any metric name).
export async function promSeriesCount(metric: string): Promise<number> {
  const q = encodeURIComponent(`{"${metric}"}`);
  const res = await fetch(`http://127.0.0.1:${PROM_PORT}/api/v1/query?query=${q}`);
  if (!res.ok) {
    return 0;
  }
  const body = (await res.json()) as { data?: { result?: unknown[] } };
  return body.data?.result?.length ?? 0;
}

// A W3C traceparent with the sampled flag set, so the whole trace is kept and the
// caller knows the trace id up front (no search needed). Returns { header, traceId }.
export function newTraceparent(): { header: string; traceId: string } {
  const hex = (n: number) =>
    [...crypto.getRandomValues(new Uint8Array(n))].map((b) => b.toString(16).padStart(2, "0")).join("");
  const traceId = hex(16);
  return { header: `00-${traceId}-${hex(8)}-01`, traceId };
}

// waitForTrace polls until the given trace has stitched across all `expected`
// services, then returns the observed set. Fails the test on timeout.
export async function waitForTraceServices(traceId: string, expected: string[]): Promise<string[]> {
  let seen: string[] = [];
  await expect
    .poll(async () => {
      seen = await tempoTraceServices(traceId);
      return expected.every((s) => seen.includes(s));
    }, { timeout: 60_000, intervals: [1000, 2000, 3000] })
    .toBe(true);
  return seen;
}
