// Panel route-group layout (ADR-0400). Exists to place the client-state
// boundary at the group that uses it rather than at the root.
import type { ReactNode } from "react";
import { PanelProviders } from "./providers";

// Every route under this group is a server component fetching live data through
// the edge, so none of them can be prerendered at build time. Without this the
// image build itself fails — `next build` exports the routes, the fetcher has no
// edge origin to call, and the error names the missing variable rather than the
// prerender that should never have run. Supplying that origin at build time is
// not the alternative: it would bake one environment's host into an image meant to
// be promoted by digest (ADR-0103), which is why the build arg is empty by design.
export const dynamic = "force-dynamic";

export default function PanelLayout({ children }: { children: ReactNode }) {
  return <PanelProviders>{children}</PanelProviders>;
}
