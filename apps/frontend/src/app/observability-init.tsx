// Loads browser observability off the critical path (ADR-0400, ADR-0500).
//
// The import is dynamic and deliberately so. A static import of
// `@/lib/observability/client` puts the Faro web SDK and its OpenTelemetry
// tracing instrumentation in the initial chunk graph — ~180 KiB parsed and
// executed before hydration finishes, on every route, including a landing page
// that renders a heading and six links. Moving `initializeFaro` into a
// `useEffect` does NOT help on its own: the effect defers when the code RUNS,
// while the static import decides when it is DOWNLOADED and PARSED, and it is
// the parse under Lighthouse's 4× CPU throttle that shows up as render delay.
//
// Scheduling then holds the fetch until the page has finished loading AND the
// main thread goes idle. Both halves matter. `requestIdleCallback` alone fires as
// soon as there is a gap, which on a fast page is while the browser is still
// settling — the SDK's parse lands inside the FCP→TTI window and shows up as Total
// Blocking Time, which ADR-0400 gates at 200 ms. Waiting for `load` first moves
// that parse past the work the budget is actually protecting. Safari has no
// `requestIdleCallback`, hence the timeout fallback.
"use client";

import { useEffect } from "react";

export function ObservabilityInit() {
  useEffect(() => {
    let cancelled = false;
    const load = () => {
      if (cancelled) {
        return;
      }
      // Failing to load RUM must never break the page it is measuring, so the
      // rejection is swallowed deliberately — there is nowhere to report it to
      // when the thing that reports is what failed to load.
      import("@/lib/observability/client")
        .then(({ initBrowserObservability }) => {
          if (!cancelled) {
            initBrowserObservability();
          }
        })
        .catch(() => undefined);
    };

    let idleHandle: number | undefined;
    let timer: number | undefined;
    const schedule = () => {
      const idle = window.requestIdleCallback;
      if (typeof idle === "function") {
        idleHandle = idle(load, { timeout: 5000 });
      } else {
        timer = window.setTimeout(load, 2000);
      }
    };

    if (document.readyState === "complete") {
      schedule();
    } else {
      window.addEventListener("load", schedule, { once: true });
    }

    return () => {
      cancelled = true;
      window.removeEventListener("load", schedule);
      if (idleHandle !== undefined) {
        window.cancelIdleCallback?.(idleHandle);
      }
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, []);
  return null;
}
