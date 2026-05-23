import Link from "next/link";
import { panel } from "@ui/strings/panel";

export default function PanelHome() {
  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-2xl font-semibold">{panel.home.title}</h1>
      <ul className="mt-4 space-y-2">
        <li>
          <Link href="/panel/products" className="text-brand-600 hover:underline">
            {panel.home.products}
          </Link>
        </li>
        <li>
          <Link href="/panel/checkout" className="text-brand-600 hover:underline">
            {panel.home.checkout}
          </Link>
        </li>
      </ul>
    </main>
  );
}
