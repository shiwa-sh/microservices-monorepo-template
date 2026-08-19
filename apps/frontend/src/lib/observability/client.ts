// Browser observability init (ADR-0500, ADR-0400). OpenTelemetry-JS web SDK
// for traces (joining the upstream trace via traceparent) plus Grafana Faro
// for RUM, errors, and Web Vitals. Both ship to the cluster's OTel Collector
// via a Traefik-fronted ingest route at /api/rum (a vendor-neutral path — the
// agent behind it can change without breaking the URL).
//
// This module is loaded through a dynamic import from observability-init.tsx, so
// nothing in the initial graph may import it statically — see log.ts, which holds
// the `obsLog` helpers precisely so `error.tsx` can reach them without dragging
// the SDK onto the critical path.
"use client";

import { getWebInstrumentations, initializeFaro } from "@grafana/faro-web-sdk";
import { TracingInstrumentation } from "@grafana/faro-web-tracing";

const INGEST = "/api/rum";
const FIRST_PARTY_API = /\/api\//;

let initialized = false;

export function initBrowserObservability(): void {
  if (initialized || typeof window === "undefined") {
    return;
  }
  initialized = true;

  initializeFaro({
    url: INGEST,
    app: {
      name: "frontend",
      version: process.env.NEXT_PUBLIC_SERVICE_VERSION ?? "dev",
      environment: process.env.NEXT_PUBLIC_DEPLOY_ENV ?? "dev",
    },
    instrumentations: [
      ...getWebInstrumentations(),
      new TracingInstrumentation({
        instrumentationOptions: {
          propagateTraceHeaderCorsUrls: [FIRST_PARTY_API],
        },
      }),
    ],
  });
}
