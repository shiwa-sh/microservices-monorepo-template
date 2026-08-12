// Kratos self-service error flow renderer (ADR-0304, ADR-0400). Kratos redirects
// here (selfservice.flows.error.ui_url) with ?id=<error>; we read the detail from
// the Kratos public API (same origin via Traefik, /auth/self-service/errors) and
// show it instead of Kratos' hosted fallback page. Client component: the id and
// session cookie only exist in the browser.
"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { landing } from "@/strings/landing";

type KratosError = {
  id?: string;
  code?: number;
  status?: string;
  reason?: string;
  message?: string;
};

const strings = landing.errorFlow;

export function AuthError() {
  const [detail, setDetail] = useState<KratosError | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("id");
    if (!id) {
      setLoading(false);
      return;
    }
    fetch(`/auth/self-service/errors?id=${encodeURIComponent(id)}`, {
      headers: { accept: "application/json" },
      credentials: "include",
    })
      .then((res) => (res.ok ? (res.json() as Promise<{ error?: KratosError }>) : null))
      .then((data) => setDetail(data?.error ?? null))
      .catch(() => setDetail(null))
      .finally(() => setLoading(false));
  }, []);

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-2xl font-semibold">{strings.title}</h1>
      {loading ? (
        <p className="mt-2 text-tertiary">{strings.loading}</p>
      ) : (
        <p className="mt-2 text-red-600">{detail?.reason ?? detail?.message ?? strings.generic}</p>
      )}
      <Link href="/auth/login" className="mt-4 block text-sm text-brand-600 hover:underline">
        {strings.toLogin}
      </Link>
    </main>
  );
}
