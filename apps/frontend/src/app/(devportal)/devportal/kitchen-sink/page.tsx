// Visual sanity check for src/components/ui primitives. The cheap alternative
// to a Storybook install: one page that renders every primitive once. Gated by
// the (devportal) Kratos session check (proxy.ts).
//
// Add one <Section> per primitive added to @/components/ui.
import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Button } from "@/components/ui";

export const metadata: Metadata = { title: "UI kitchen sink" };

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="border-t border-slate-200 py-6 first:border-t-0">
      <h2 className="text-lg font-semibold text-slate-900">{title}</h2>
      <div className="mt-3 flex flex-wrap items-center gap-3">{children}</div>
    </section>
  );
}

export default function KitchenSink() {
  return (
    <main className="mx-auto max-w-3xl p-6">
      <header>
        <h1 className="text-2xl font-semibold">UI kitchen sink</h1>
        <p className="mt-1 text-sm text-slate-600">
          Every primitive in <code>src/components/ui</code>, rendered once. Visual sanity check for
          the Untitled UI bump cadence; not a replacement for component tests.
        </p>
      </header>

      <Section title="Button">
        <Button>Primary</Button>
        <Button variant="secondary">Secondary</Button>
        <Button variant="ghost">Ghost</Button>
        <Button disabled>Disabled</Button>
      </Section>
    </main>
  );
}
