// Client-side providers (ADR-0014). TanStack Query for client-cached SDK
// calls; next-themes for dark-mode; nuqs for URL state; OpenFeature client
// init. Browser observability is initialised in observability-init.tsx so it
// runs once on mount without blocking SSR.
"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { NuqsAdapter } from "nuqs/adapters/next/app";
import { type ReactNode, useState } from "react";

export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient());
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider attribute="data-theme" defaultTheme="system">
        <NuqsAdapter>{children}</NuqsAdapter>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
