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
import { fingerprint } from "@libs/observability";

const INGEST = "/api/rum";
const FIRST_PARTY_API = /\/api\//;

let initialized = false;

type ExceptionPayload = {
  type?: string;
  value?: string;
  stacktrace?: { frames?: Array<{ filename?: string; function?: string }> };
  context?: Record<string, string>;
};

// Attaches the fault identifier to every exception on its way out (ADR-0503).
//
// A `beforeSend` hook rather than a call at each `pushError` site: an error
// reaches Faro from the window handler, from an unhandled rejection, and from
// `obsLog` — three paths, and a fingerprint added at two of them is a dashboard
// that quietly under-reports the third. This is the one place every item passes.
//
// It goes in `context`, which Faro carries as attributes. Never a label: the value
// is high-cardinality by construction and must not reach a metric or a Loki stream
// label (ADR-0500).
function withFingerprint<T extends { type: string; payload: unknown }>(item: T): T {
  if (item.type !== "exception") {
    return item;
  }
  const payload = item.payload as ExceptionPayload;
  const stack = (payload.stacktrace?.frames ?? [])
    .map((frame) => `    at ${frame.function ?? "?"} (${frame.filename ?? "?"})`)
    .join("\n");
  payload.context = {
    ...payload.context,
    "error.fingerprint": fingerprint({ name: payload.type, message: payload.value, stack }),
  };
  return item;
}

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
    // Attach the fault identifier to every exception on its way out (ADR-0503).
    //
    // Here rather than at each `pushError` call site: an error reaches Faro from
    // the window handler, from an unhandled rejection, and from `obsLog` — three
    // paths, and a fingerprint added at two of them is a dashboard that quietly
    // under-reports one. `beforeSend` is the single place every item passes.
    //
    // It is `context`, which Faro carries as attributes, not a label: the value is
    // high-cardinality by construction and must never become a metric or stream
    // label (ADR-0500).
    beforeSend: withFingerprint,
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
  //
  // `isSampled` is not decoration, and it is the one attribute this session cannot
  // go without. Faro's tracing sampler asks the SESSION whether it is sampled and
  // records nothing when the attribute is absent (@grafana/faro-web-tracing
  // `getSamplingDecision`) — so an unset flag does not merely drop browser spans,
  // it stamps `traceparent: …-00` on every request the page makes to the API, and
  // the services' parentbased sampler honours it. One missing attribute therefore
  // deletes the whole server-side trace of anything a user did, end to end, and the
  // signal that normally exposes a sampling mistake — a trace that is thin — never
  // appears, because there is no trace at all.
  //
  // The rate that keeps volume down stays where ADR-0500 puts it, in the collector's
  // head sampler. This says only that the browser has no opinion to enforce.
  faro.api.setSession({ id: crypto.randomUUID(), attributes: { isSampled: "true" } });
}
