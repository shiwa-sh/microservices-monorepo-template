// Observability wiring gauge (ADR-0011): the three Grafana datasources resolve
// and answer. This is the acceptance check behind the service-name fix — Grafana,
// the OTel collector and prod all reach Loki/Tempo/Mimir at their short in-cluster
// names (loki/tempo/mimir, not the release-prefixed defaults), and Loki runs
// single-tenant so header-less queries don't 401. Driven through the real ops
// edge with the saved AAL2 operator session (Grafana's HTTP API, no browser).
import { type APIRequestContext, expect, request, test } from "@playwright/test";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const GRAFANA_API = `${opsURL("grafana")}/api/datasources`;

test.describe("grafana datasources", () => {
  for (const name of ["Loki", "Tempo", "Mimir"]) {
    test(`${name} datasource is healthy behind the operator session`, async () => {
      const ctx: APIRequestContext = await request.newContext({
        ignoreHTTPSErrors: true,
        storageState: OPERATOR_STATE,
      });
      try {
        const ds = await ctx.get(`${GRAFANA_API}/name/${name}`);
        expect(ds.ok(), `${name} datasource is provisioned`).toBeTruthy();
        const { uid } = await ds.json();
        const health = await ctx.get(`${GRAFANA_API}/uid/${uid}/health`);
        expect(health.ok(), `${name} health endpoint reachable`).toBeTruthy();
        expect((await health.json()).status, `${name} reachable from Grafana`).toBe("OK");
      } finally {
        await ctx.dispose();
      }
    });
  }
});
