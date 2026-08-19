// Root providers (ADR-0400) — theming only, and that limit is the point.
//
// next-themes writes Untitled's class onto the root element, so it has to wrap
// every route or an unstyled flash appears on the ones it misses. TanStack Query
// and nuqs do not: they serve the interactive surface, and ADR-0400 scopes them
// to client components reading through the generated SDKs and to URL state for
// filters, pagination and tabs. Mounting them here instead put their runtime in
// the initial chunk graph of every route — including a landing page and an auth
// form that use neither — which is bundle weight and hydration work spent on
// nothing, against the LCP budget ADR-0400 gates merges on.
//
// They now live in `(panel)/providers.tsx`. A route group that grows a need for
// them gets its own copy of that boundary; the root is not the place to add one.
"use client";

import { ThemeProvider } from "next-themes";
import type { ReactNode } from "react";

export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider
      attribute="class"
      value={{ light: "light-mode", dark: "dark-mode" }}
      defaultTheme="system"
      enableSystem
    >
      {children}
    </ThemeProvider>
  );
}
