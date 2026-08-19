// The sign-in link on the 401 fallback (ADR-0400). A client component for one
// reason: it needs the URL that was denied, and only the browser has it here.
// `unauthorized()` renders in place without changing the address, so
// window.location still names the page the user was on — which is what makes
// "you will come back to this page" true rather than a hopeful phrase.
//
// Path AND query: the panel keeps filters, pagination and tab selection in the URL
// (nuqs), and a return_to of the bare pathname returns a screen the user never left.
"use client";

import type { Route } from "next";
import Link from "next/link";
import { useEffect, useState } from "react";

export function SignInAgain() {
  // Read after mount, not during render: the server pass has no location, and
  // deriving the href from it would mismatch on hydration.
  const [href, setHref] = useState("/auth/login");

  useEffect(() => {
    const returnTo = `${window.location.pathname}${window.location.search}`;
    setHref(`/auth/login?return_to=${encodeURIComponent(returnTo)}`);
  }, []);

  return (
    // typedRoutes cannot check a template string; the shape is asserted above.
    <Link href={href as Route} className="text-sm text-brand-600 hover:underline">
      Sign in again
    </Link>
  );
}
