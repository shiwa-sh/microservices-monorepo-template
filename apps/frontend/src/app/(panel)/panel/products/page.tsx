// Server component fetches via the server-only fetcher (ADR-0400). A direct
// fetch() to a service URL is lint-forbidden; this goes through the generated
// catalog SDK, whose `paths` type is the spec rather than a copy of it.
import type { paths } from "@sdks/catalog";
import { createServerClient } from "@/lib/server-fetch/server";
import { panel } from "@/strings/panel";
// React Aria's Table must build its collection client-side, so the interactive
// table is a client child; this RSC just fetches and hands it the data.
import { ProductsTable } from "./products-table";

export default async function Products() {
  const catalog = await createServerClient<paths>();
  const { data } = await catalog.GET("/products");
  const products = data ?? [];

  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-2xl font-semibold">{panel.products.title}</h1>
      <ProductsTable products={products} />
    </main>
  );
}
