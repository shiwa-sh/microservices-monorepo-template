import "@/styles/globals.css";

import type { ReactNode } from "react";
import { ObservabilityInit } from "./observability-init";
import { Providers } from "./providers";

export const metadata = { title: "Platform" };

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="min-h-screen bg-white text-slate-900 antialiased">
        <Providers>
          <ObservabilityInit />
          {children}
        </Providers>
      </body>
    </html>
  );
}
