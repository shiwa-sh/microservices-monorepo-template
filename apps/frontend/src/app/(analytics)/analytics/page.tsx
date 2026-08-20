// The funnel surface (ADR-0700).
//
// Reads the analytics service east-west: it is `x-audience: cluster` and has no
// `/api` route, so there is no path from a browser to the event store at all.
// Everything rendered here arrives through this server component.
import { createInternalClient } from "@/lib/server-fetch/internal";

type AnalyticsPaths = {
  "/analytics/summary": {
    get: {
      parameters: { query: { since: string } };
      responses: {
        200: {
          content: {
            "application/json": Array<{ name: string; occurrences: number; sessions: number }>;
          };
        };
      };
    };
  };
};

const WINDOW_DAYS = 7;

export default async function AnalyticsPage() {
  const analytics = createInternalClient<AnalyticsPaths>("ANALYTICS_URL");
  const since = new Date(Date.now() - WINDOW_DAYS * 24 * 60 * 60 * 1000).toISOString();
  const { data } = await analytics.GET("/analytics/summary", { params: { query: { since } } });
  const rows = data ?? [];

  return (
    <main>
      <h1 className="text-2xl font-semibold">Product analytics</h1>
      <p className="mt-2 text-sm">
        Events recorded in the last {WINDOW_DAYS} days. Sessions is the number that answers “how
        many people”; occurrences answers “how many times”.
      </p>
      {rows.length === 0 ? (
        <p className="mt-6 text-sm">
          No events in this window. Marketing events are only recorded for sessions that granted
          consent, so an empty table is also what “nobody consented yet” looks like.
        </p>
      ) : (
        <table className="mt-6 w-full text-left text-sm">
          <thead>
            <tr>
              <th className="py-2">Event</th>
              <th className="py-2">Occurrences</th>
              <th className="py-2">Sessions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.name}>
                <td className="py-1">{row.name}</td>
                <td className="py-1">{row.occurrences}</td>
                <td className="py-1">{row.sessions}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
