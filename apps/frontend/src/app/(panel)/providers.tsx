// Panel providers (ADR-0400). The interactive surface, and the only route group
// that needs any of these — see the note in `src/app/providers.tsx` for why
// they are not at the root.
//
//   - TanStack Query: client components read through the generated SDKs here,
//     which is the refetch/invalidate model ADR-0400 chose it for.
//   - nuqs: URL state for the panel's filters, pagination and tab selection.
"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NuqsAdapter } from "nuqs/adapters/next/app";
import { type ReactNode, useState } from "react";
import { isAuthDenial } from "@/lib/auth/denial";

// The group's access-denial handling, in one place, so no page carries an auth
// branch (ADR-0400).
//
// The browser fetch client throws Next's 401/403 interrupt (lib/auth/denial.ts).
// Those only reach `unauthorized.tsx` / `forbidden.tsx` if they surface DURING
// RENDER, and TanStack Query does the opposite by design: it catches whatever the
// query function throws and hands it back as `error` state. Left alone, every
// denial in the panel would end up as a component-local "something went wrong"
// that each developer re-invents, or as nothing at all.
//
// `throwOnError` re-throws it during the render of the component that owns the
// query or mutation — but only for a denial. Ordinary failures keep the normal
// `error` state, because a 500 from one widget should not take the page down.
//
// This is why ADR-0400 requires browser API calls to go through TanStack Query
// rather than a bare call in an event handler: React boundaries do not catch what
// an event handler throws, so a denial raised there becomes an unhandled rejection
// and the user sees nothing at all. Going through a query or a mutation is what
// gives the interrupt a render pass to surface in.
function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { throwOnError: isAuthDenial },
      mutations: { throwOnError: isAuthDenial },
    },
  });
}

export function PanelProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(makeQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <NuqsAdapter>{children}</NuqsAdapter>
    </QueryClientProvider>
  );
}
