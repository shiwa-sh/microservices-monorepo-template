// Panel providers (ADR-0400). The interactive surface, and the only route group
// that needs either of these — see the note in `src/app/providers.tsx` for why
// they are not at the root.
//
//   - TanStack Query: client components read through the generated SDKs here,
//     which is the refetch/invalidate model ADR-0400 chose it for.
//   - nuqs: URL state for the panel's filters, pagination and tab selection.
"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NuqsAdapter } from "nuqs/adapters/next/app";
import { type ReactNode, useState } from "react";

export function PanelProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient());
  return (
    <QueryClientProvider client={queryClient}>
      <NuqsAdapter>{children}</NuqsAdapter>
    </QueryClientProvider>
  );
}
