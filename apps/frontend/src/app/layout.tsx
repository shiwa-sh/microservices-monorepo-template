import "@/styles/globals.css";

import type { Metadata } from "next";
import { Inter } from "next/font/google";
import type { ReactNode } from "react";
import { ObservabilityInit } from "./observability-init";
import { Providers } from "./providers";

export const metadata: Metadata = { title: "Platform" };

// Untitled UI's --font-inter hook (ADR-0400). theme.css falls back to system
// fonts if this is absent; next/font is the sanctioned font loader.
const inter = Inter({ subsets: ["latin"], variable: "--font-inter", display: "swap" });

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className={inter.variable} suppressHydrationWarning>
      <body className="min-h-screen bg-primary text-primary antialiased">
        <Providers>
          <ObservabilityInit />
          {children}
        </Providers>
      </body>
    </html>
  );
}
