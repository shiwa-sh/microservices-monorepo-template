"use client";

// React Aria's Table builds its collection by introspecting Row/Cell children,
// which only works client-side — a Server Component hands it opaque client
// references and the collection renders empty/undefined. So the RSC page fetches
// the data and this client child renders the interactive Table from it.
import { formatMoney, type Money } from "@libs/money";

import { Table } from "@/components/application/table/table";

export type Product = { id: string; name: string; price: Money };

export function ProductsTable({ products }: { products: Product[] }) {
  return (
    <Table aria-label="Products" className="mt-4">
      <Table.Header>
        <Table.Head id="name" isRowHeader label="Product" />
        <Table.Head id="price" label="Price" />
      </Table.Header>
      <Table.Body>
        {products.map((p) => (
          <Table.Row key={p.id} id={p.id}>
            <Table.Cell className="font-medium text-primary">{p.name}</Table.Cell>
            {/* formatMoney, not `/ 100` and a hardcoded `$`: the minor digits and the
                symbol belong to the currency, and the amount is a decimal string
                precisely so it never passes through a double (ADR-0300). */}
            <Table.Cell className="tabular-nums">{formatMoney(p.price)}</Table.Cell>
          </Table.Row>
        ))}
      </Table.Body>
    </Table>
  );
}
