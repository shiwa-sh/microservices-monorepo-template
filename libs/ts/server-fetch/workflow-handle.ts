// Cross-service mutation polling (ADR-0006, ADR-0014). Services that start a
// workflow respond with 202 + a handle conforming to api/shared/workflow-handle.yaml.
// This helper polls the status endpoint until the workflow terminates.
export type WorkflowHandle = {
  id: string;
  status_url: string;
};

export type WorkflowStatus<T> = {
  status: "running" | "succeeded" | "failed";
  result?: T;
  error?: string;
};

type PollOpts = {
  intervalMs?: number;
  signal?: AbortSignal;
};

export async function pollWorkflow<T>(
  handle: WorkflowHandle,
  { intervalMs = 1000, signal }: PollOpts = {},
): Promise<WorkflowStatus<T>> {
  while (true) {
    if (signal?.aborted) throw new Error("aborted");
    const res = await fetch(handle.status_url, { cache: "no-store", signal });
    const body = (await res.json()) as WorkflowStatus<T>;
    if (body.status !== "running") return body;
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
}
