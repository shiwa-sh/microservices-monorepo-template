// Liveness/readiness endpoint for the in-cluster frontend (ADR-0500, ADR-0205).
//
// The Go services answer /livez and /readyz on a separate admin port; a Next
// standalone server has one port and no such surface, so the chart's probes point
// here instead (infra/gitops/services/*/values/frontend.yaml).
//
// Deliberately SHALLOW — it reports that this process is serving, nothing more. It
// does not ping the edge or any service: readiness that depends on downstreams
// turns one slow dependency into a rolling restart of a healthy frontend, which is
// the failure mode ADR-0500 calls out for liveness and which matters just as much
// here, where the "dependency" is the whole API.
export const dynamic = "force-dynamic";

export function GET() {
  return Response.json({ status: "ok" });
}
