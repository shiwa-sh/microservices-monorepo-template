// Analytics route-group layout (ADR-0700).
//
// The group exists to put the authorization check in ONE place. ADR-0700 gates
// this panel on a relationship rather than on a session, and a check written per
// page is a check that is missing from the page someone adds next month.
//
// It is a route group on the product origin rather than a separate app or an ops
// origin, and that placement is the decision that resolves who marketing are:
// they read funnels without an operator account, without operator MFA, and without
// reaching Grafana — while the ops tier's least-authority rule stays intact.
import type { Metadata } from "next";
import type { ReactNode } from "react";
import { requireRelation } from "@/lib/auth/panel-access";

// `noindex` because the pages sit on the PRODUCT origin, which is the origin
// crawlers are invited to. The authorization check below is what actually
// protects them — this only keeps them out of results.
export const metadata: Metadata = {
  robots: { index: false, follow: false },
};

// Every render asks. Caching the answer would cache an authorization decision,
// which is the one thing that must reflect the request being served: a grant
// withdrawn a minute ago has to take effect on the next page load.
export const dynamic = "force-dynamic";

export default async function AnalyticsLayout({ children }: { children: ReactNode }) {
  await requireRelation("view", "analytics_panel:funnels");
  return <section className="mx-auto max-w-4xl p-6">{children}</section>;
}
