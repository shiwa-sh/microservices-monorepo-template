// Browser observability init (ADR-0011, ADR-0014). OpenTelemetry-JS web SDK
// for traces (joining the upstream trace via traceparent) plus Grafana Faro
// for RUM, errors, and Web Vitals. Both ship to the cluster's OTel Collector
// via a Traefik-fronted ingest route at /api/observability/*.
"use client";

import { getWebInstrumentations, initializeFaro } from "@grafana/faro-web-sdk";
import { TracingInstrumentation } from "@grafana/faro-web-tracing";

const INGEST = "/api/observability";

let initialized = false;

export function initBrowserObservability(): void {
  if (initialized || typeof window === "undefined") return;
  initialized = true;

  initializeFaro({
    url: `${INGEST}/faro/collect`,
    app: {
      name: "frontend",
      version: process.env.NEXT_PUBLIC_SERVICE_VERSION ?? "dev",
      environment: process.env.NEXT_PUBLIC_DEPLOY_ENV ?? "dev",
    },
    instrumentations: [
      ...getWebInstrumentations(),
      new TracingInstrumentation({
        instrumentationOptions: {
          propagateTraceHeaderCorsUrls: [/\/api\//],
        },
      }),
    ],
  });
}

type Loggable = string | number | boolean | null | undefined | object;

export const obsLog = {
  info: (msg: string, ctx?: Record<string, Loggable>) =>
    window.faro?.api.pushLog([msg], { context: ctx }),
  warn: (msg: string, ctx?: Record<string, Loggable>) =>
    window.faro?.api.pushLog([msg], { context: ctx, level: "warn" }),
  error: (err: Error, ctx?: Record<string, Loggable>) =>
    window.faro?.api.pushError(err, { context: ctx }),
};

declare global {
  interface Window {
    faro?: {
      api: {
        pushLog: (args: unknown[], opts?: { context?: object; level?: string }) => void;
        pushError: (err: Error, opts?: { context?: object }) => void;
      };
    };
  }
}
