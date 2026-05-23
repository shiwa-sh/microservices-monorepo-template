// Server component fetches via the server-only fetcher (ADR-0014). Direct
// fetch() to service URLs is lint-forbidden; this path goes through the
// generated catalog SDK once `mise run gen:openapi` has produced it.
//
// import type { paths } from "@sdks/catalog";
// const catalog = await createServerClient<paths>({ service: "catalog" });
// const { data } = await catalog.GET("/products");

import { createServerClient } from "@server-fetch";
import { panel } from "@ui/strings/panel";

type Product = { id: string; name: string; price_cents: number };

type CatalogPaths = {
  "/products": { get: { responses: { 200: { content: { "application/json": Product[] } } } } };
};

export default async function Products() {
  const catalog = await createServerClient<CatalogPaths>({ service: "catalog" });
  const { data } = await catalog.GET("/products");
  const products = data ?? [];

  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-2xl font-semibold">{panel.products.title}</h1>
      <ul className="mt-4 divide-y divide-slate-200">
        {products.map((p) => (
          <li key={p.id} className="flex justify-between py-2">
            <span>{p.name}</span>
            <span className="tabular-nums text-slate-600">
              ${(p.price_cents / 100).toFixed(2)}
            </span>
          </li>
        ))}
      </ul>
    </main>
  );
}
