// Dev-only RUM ingest shim (ADR-0500, ADR-0400). In the cluster, Traefik routes
// /api/rum to the OTel Collector's Faro receiver and this Next pod never sees the
// path, so in production this handler 404s to match. Locally there is no edge:
// with FARO_COLLECT_URL set we forward beacons to a local Grafana Alloy
// faro.receiver (`mise run dev:faro`); without it we swallow them so the dev
// console isn't spammed with 404s.
import { type NextRequest, NextResponse } from "next/server";

const COLLECT_URL = process.env.FARO_COLLECT_URL;

export async function POST(req: NextRequest): Promise<NextResponse> {
  if (process.env.NODE_ENV === "production") {
    return new NextResponse(null, { status: 404 });
  }
  if (!COLLECT_URL) {
    return new NextResponse(null, { status: 204 });
  }
  try {
    const res = await fetch(COLLECT_URL, {
      method: "POST",
      headers: { "content-type": req.headers.get("content-type") ?? "application/json" },
      body: await req.text(),
    });
    return new NextResponse(null, { status: res.ok ? 204 : 502 });
  } catch {
    return new NextResponse(null, { status: 502 });
  }
}
