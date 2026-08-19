// Panel route-group layout (ADR-0400). Exists to place the client-state
// boundary at the group that uses it rather than at the root.
import type { ReactNode } from "react";
import { PanelProviders } from "./providers";

export default function PanelLayout({ children }: { children: ReactNode }) {
  return <PanelProviders>{children}</PanelProviders>;
}
