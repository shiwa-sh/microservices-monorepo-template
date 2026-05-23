// Server-only fetcher (ADR-0014). Wraps openapi-fetch so server components
// call services with the Kratos session cookie and W3C trace context attached.
// Client components must use ./client.ts.
import "server-only";

import { headers } from "next/headers";
import createClient, { type Client } from "openapi-fetch";

type CreateOpts = {
  service: string;
};

const API_BASE = process.env.INTERNAL_API_BASE ?? "http://gateway.platform.svc.cluster.local";

export async function createServerClient<Paths extends object>({
  service,
}: CreateOpts): Promise<Client<Paths>> {
  const h = await headers();
  const cookie = h.get("cookie") ?? "";
  const traceparent = h.get("traceparent") ?? "";

  return createClient<Paths>({
    baseUrl: `${API_BASE}/api/${service}`,
    headers: {
      ...(cookie && { cookie }),
      ...(traceparent && { traceparent }),
    },
  });
}
