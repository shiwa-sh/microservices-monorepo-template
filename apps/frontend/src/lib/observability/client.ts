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

  const faro = initializeFaro({
    url: INGEST,
    app: {
      name: "frontend",
      version: process.env.NEXT_PUBLIC_SERVICE_VERSION ?? "dev",
      environment: process.env.NEXT_PUBLIC_DEPLOY_ENV ?? "dev",
    },
    // OFF, and this is a legal position rather than a tuning choice (ADR-0400).
    //
    // Faro's default is `{ enabled: true, persistent: false }`, and `persistent:
    // false` does not mean "in memory" — it selects VolatileSessionsManager, which
    // writes `com.grafana.faro.session` into sessionStorage and resumes it for up
    // to four hours (read off the SDK's own source at 2.7.1). That is storage in
    // the user's terminal equipment, which puts the ops path inside ePrivacy Art.
    // 5(3) and therefore behind the consent gate ADR-0700 builds for analytics —
    // for a signal that exists to tell us the app is broken.
    //
    // With this off, the SDK touches no web storage at all: the branch that calls
    // `storeUserSession` is the one this flag guards. The session id below replaces
    // it, in memory, for the life of the page.
    sessionTracking: { enabled: false },
    instrumentations: [
      ...getWebInstrumentations(),
      new TracingInstrumentation({
        instrumentationOptions: {
          propagateTraceHeaderCorsUrls: [FIRST_PARTY_API],
        },
      }),
    ],
  });

  // A per-page correlation id, held in a closure and nowhere else. It survives
  // exactly as long as the page does: a reload is a new id, which is the property
  // that keeps it out of Art. 5(3) — nothing is stored, so nothing is read back.
  // It is enough to stitch one visit's errors and Web Vitals together, which is
  // what the RUM path is for.
  faro.api.setSession({ id: crypto.randomUUID() });
}
