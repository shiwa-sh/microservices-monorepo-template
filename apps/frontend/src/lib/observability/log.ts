// Browser log/error forwarding (ADR-0500, ADR-0400).
//
// Deliberately separate from `client.ts`, and deliberately importing nothing.
// `client.ts` pulls in the Faro web SDK and its tracing instrumentation, which
// carry OpenTelemetry — ~180 KiB of the landing page's initial JS when anything
// in the initial graph reaches it. `error.tsx` is in every route's graph, so a
// single `obsLog` import there was enough to put the whole SDK on the critical
// path of a page that is a heading and six links.
//
// These functions only read `window.faro`, which `initBrowserObservability`
// stamps on once it has loaded. Before that they are silent no-ops — correct,
// because there is nowhere to send a log until the SDK is up.
"use client";

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
  // biome-ignore lint/style/useConsistentTypeDefinitions: global augmentation must use `interface`
  interface Window {
    faro?: {
      api: {
        pushLog: (args: unknown[], opts?: { context?: object; level?: string }) => void;
        pushError: (err: Error, opts?: { context?: object }) => void;
      };
    };
  }
}
