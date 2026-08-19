// 403 fallback (ADR-0400). Rendered by `forbidden()` from lib/auth/denial.ts.
//
// It renders AT THE DENIED URL and does not redirect, which is the entire point.
// The user is signed in; signing in again is not the remedy, and sending them to
// the login flow would loop them straight back here. What they need is the address
// they were denied — it is what they hand to whoever can grant access — and a
// back button that still goes back.
//
// Nothing here names the resource or the missing permission: this file is shared
// by every route, and the frontend does not hold the decision. The service does
// (OpenFGA, ADR-0304), and a resource the user must not know exists answers 404
// rather than 403, so there is nothing to disclose here either way.
//
// Known and measured: the RESPONSE STATUS is 200, not 403, on any route behind
// the root loading.tsx — which is every route. The status line is sent before the
// body, and the Suspense boundary starts the body as soon as the fallback renders,
// so by the time a server component's fetch answers, the status is already spent
// and the denial arrives inside the stream. Next documents this on loading.js
// (§ Status Codes) and its remedy is to decide before anything streams — which is
// the proxy, and is where the SESSION check already lives. A permission cannot
// move there: the frontend holds no permission data (ADR-0304).
//
// Verified against the standalone build, in both directions and against a control:
//
//   - with loading.tsx    → 200, loading indicator paints, then this page
//   - without loading.tsx → a true 403, and an empty body until hydration
//   - notFound() after an await, without loading.tsx → 404, also an empty body
//
// The third line is why this is not an authorization bug to fix. Any interrupt
// raised after an await renders on the client; the Suspense boundary only decides
// whether the user watches a loading indicator or a blank page while that happens.
// Keeping the boundary trades a status line that only machines read for a screen
// that is never blank, and keeps the LCP decision in ADR-0400 intact.
import Link from "next/link";

export default function Forbidden() {
  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-xl font-semibold text-primary">You do not have access</h1>
      <p className="mt-2 text-sm text-tertiary">
        Your account is signed in but is not permitted to view this. Ask whoever administers your
        organisation for access — the address in your browser bar is the one to send them.
      </p>
      <div className="mt-4 flex gap-4 text-sm">
        <Link href="/panel" className="text-brand-600 hover:underline">
          Back to the panel
        </Link>
        <Link href="/" className="text-brand-600 hover:underline">
          Home
        </Link>
      </div>
    </main>
  );
}
