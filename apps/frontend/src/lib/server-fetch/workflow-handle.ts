// Cross-service mutation polling (ADR-0302, ADR-0400). Services that start a
// workflow respond with 202 + a WorkflowHandle (the schema declared in each
// service's openapi.yaml): { id, run_id, status, result_url }. The handle's
// result_url points at the resource's GET endpoint (e.g. /api/orders/<id>), whose
// payload carries the terminal domain status. This helper polls that URL until the
// resource reaches a terminal state.
export type WorkflowHandle = {
  id: string;
  run_id: string;
  // Temporal run status at enqueue time; always "running" for a fresh handle.
  status: "running" | "completed" | "failed" | "cancelled";
  // GET to fetch the terminal status + result. Omitted only by handles that carry
  // the result inline (none today).
  result_url?: string;
};

import { raiseForAuthDenial } from "@/lib/auth/denial";

// The terminal-state shape shared by the resources a workflow settles (orders,
// charges): a domain status string. "pending"/"running" are non-terminal.
export type TerminalResource = { status: string };

const TERMINAL = new Set(["confirmed", "completed", "failed", "cancelled", "settled", "refunded"]);

type PollOpts = {
  intervalMs?: number;
  timeoutMs?: number;
  // How long a 403 on the polled resource is read as "the grant has not landed
  // yet" rather than as a denial. See the note in the loop below.
  forbiddenGraceMs?: number;
  signal?: AbortSignal;
};

// pollWorkflow GETs the handle's result_url until the resource's status is terminal
// (or the timeout elapses), then returns that resource. It resolves on the first
// terminal status regardless of success/failure — the caller decides how to render
// "failed" vs "confirmed" (a failed charge is still a completed poll, not an error).
export async function pollWorkflow<T extends TerminalResource>(
  handle: WorkflowHandle,
  { intervalMs = 1000, timeoutMs = 60_000, forbiddenGraceMs = 10_000, signal }: PollOpts = {},
): Promise<T> {
  if (!handle.result_url) {
    throw new Error("workflow handle has no result_url to poll");
  }
  const deadline = Date.now() + timeoutMs;
  const graceEnds = Date.now() + forbiddenGraceMs;
  // biome-ignore lint/suspicious/noUnnecessaryConditions: poll loop exits via return/throw below
  while (true) {
    if (signal?.aborted) {
      throw new Error("aborted");
    }
    // biome-ignore lint/performance/noAwaitInLoops: workflow polling is intentionally sequential
    const res = await fetch(handle.result_url, { cache: "no-store", signal });
    // A denial is terminal for this poll, and the loop cannot see that on its own:
    // every non-ok status is treated as "not settled yet", so a session that
    // expires mid-workflow spins silently until the timeout and then reports the
    // workflow as slow. This raises the same interrupt the fetch clients do.
    //
    // With one exception, and it is the first status this poll ever sees. The
    // resource being polled is created by the workflow being polled, and so are the
    // tuples that say who may read it — the dual write is the workflow's own first
    // leg (ADR-0304). Between the 202 and that leg the resource is genuinely
    // forbidden to the buyer who just placed it, so raising on it turns a checkout
    // that is about to succeed into "You do not have access". Measured locally the
    // window is ~100ms; the grace is two orders of magnitude wider so a loaded
    // cluster stays inside it, and a 403 that outlives it is a real denial and
    // raises exactly as before. 401 never waits: no session is no session.
    if (res.status !== 403 || Date.now() > graceEnds) {
      raiseForAuthDenial(res.status);
    }
    if (res.ok) {
      const body = (await res.json()) as T;
      if (TERMINAL.has(body.status)) {
        return body;
      }
    }
    if (Date.now() > deadline) {
      throw new Error("workflow poll timed out before reaching a terminal status");
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
}
