// Kratos session gate + per-request CSP nonce (ADR-0304, ADR-0400, ADR-0305).
// (landing) is public except for /auth/*; the other route groups require a
// session. The frontend never validates JWTs — Oathkeeper does that at the edge
// for /api/* calls. Here we only check that a Kratos session cookie is present,
// forward the session id as a header, and set a strict per-request CSP whose
// nonce the root layout applies to first-party scripts.
//
// This file handles exactly ONE half of access control: no session, at navigation
// time. That is the half a redirect fits, because signing in is the remedy and
// nothing is lost by redirecting before anything renders. It knows nothing about
// permissions, and must not: a user who is signed in and not allowed keeps their
// URL and is shown a 403 in place (src/lib/auth/denial.ts, app/forbidden.tsx).
// Redirecting that user to the login flow sends them round a loop that ends where
// it started, minus the address they needed.
import { type NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "ory_kratos_session";

// Route groups that require an authenticated Kratos session.
// `/analytics` is here for the session half only. The route group performs the
// AUTHORITATIVE check itself (ADR-0700): this redirects someone with no session to
// sign in, which is a better first experience than a 403, and the layout decides
// whether a signed-in person may actually read funnels.
const PROTECTED = ["/panel", "/devportal", "/analytics"];

// The /auth/* pages and the Kratos flow each one starts. A Kratos browser flow
// cannot begin on our side: Kratos has to set its CSRF cookie on the user's
// browser and hand back a flow id, so the browser must visit it.
//
// Issuing that redirect HERE, rather than from the page, is a measured LCP fix
// (ADR-0400). Middleware runs before rendering, so it answers with a real 307 and
// no HTML. From the page it cannot: `loading.tsx` puts the route behind a Suspense
// boundary, Next flushes the shell before the redirect is known, and the response
// is a 200 carrying an in-stream redirect — a full document render, a paint of the
// loading fallback, and only then the navigation. That fallback was measuring as
// the LCP element of /auth/login.
//
// The path segment is not always the flow name: /auth/register starts Kratos's
// `registration` flow.
const AUTH_FLOWS: Record<string, string> = {
  login: "login",
  register: "registration",
  recovery: "recovery",
  verification: "verification",
  settings: "settings",
};

// Flows that a user WITH a live session has no business starting. Kratos refuses
// them (`session_already_available`) and answers by sending the browser to
// `default_browser_return_url`, so without this the back button after a successful
// sign-in silently teleports the user to the landing page: the stack still holds
// /auth/login?flow=<consumed id>, and revisiting it starts a fresh flow.
//
// `settings` and `verification` are deliberately absent — both are meaningful
// while signed in (MFA enrolment, confirming an address), and both need a session.
const SIGNED_IN_HAS_NO_FLOW = new Set(["login", "register", "recovery"]);

// Telemetry ingest origin for connect-src. Same-origin (/api/rum via Traefik)
// by default; override when RUM ships to a distinct host.
const INGEST_ORIGIN = process.env.NEXT_PUBLIC_OTEL_INGEST_ORIGIN ?? "";

function makeNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return btoa(String.fromCharCode(...bytes));
}

function contentSecurityPolicy(nonce: string): string {
  const connectSrc = ["'self'", INGEST_ORIGIN].filter(Boolean).join(" ");
  // Production: a strict nonce + strict-dynamic policy. `next dev` can't satisfy
  // it — it injects un-nonced inline HMR/fast-refresh scripts and needs eval(),
  // and strict-dynamic makes the browser ignore 'unsafe-inline' — so the dev
  // server gets an inline-permissive policy instead. Production never uses eval
  // or un-nonced inline scripts, so it keeps the strict form.
  const scriptSrc =
    process.env.NODE_ENV === "production"
      ? ["'self'", `'nonce-${nonce}'`, "'strict-dynamic'"]
      : ["'self'", "'unsafe-inline'", "'unsafe-eval'"];
  return [
    "default-src 'self'",
    `script-src ${scriptSrc.join(" ")}`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    `connect-src ${connectSrc}`,
    "font-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
}

// A `return_to` is attacker-controllable, so only a same-site absolute path is
// ever followed: "//evil.example" is a protocol-relative URL the browser treats as
// another origin, and a bare relative path could escape the app. Kratos applies its
// own `allowed_return_urls` check on the flow side; this is the same guard on ours,
// because this function also reads the parameter back off a URL Kratos never saw.
function safeReturnTo(raw: string | null): string | null {
  if (!raw?.startsWith("/") || raw.startsWith("//")) {
    return null;
  }
  return raw;
}

export function proxy(req: NextRequest) {
  const path = req.nextUrl.pathname;
  const isProtected = PROTECTED.some((p) => path === p || path.startsWith(`${p}/`));
  const hasSession = req.cookies.get(SESSION_COOKIE) !== undefined;

  const authSegment = path.startsWith("/auth/") ? path.slice("/auth/".length) : "";
  const flowKind = AUTH_FLOWS[authSegment];

  // Already signed in, and asking for a flow that only makes sense signed out.
  // Answering here rather than letting Kratos answer keeps the user inside the app
  // and keeps whatever `return_to` the URL carries; Kratos would discard it.
  if (flowKind && hasSession && SIGNED_IN_HAS_NO_FLOW.has(authSegment)) {
    const back = safeReturnTo(req.nextUrl.searchParams.get("return_to")) ?? "/";
    return NextResponse.redirect(new URL(back, req.url));
  }

  // An /auth/* page with no flow id yet: send the browser to Kratos to start one.
  // With a flow id, fall through — the page renders it server-side.
  if (flowKind && !req.nextUrl.searchParams.get("flow")) {
    const start = new URL(`/auth/self-service/${flowKind}/browser`, req.url);
    const returnTo = safeReturnTo(req.nextUrl.searchParams.get("return_to"));
    if (returnTo) {
      start.searchParams.set("return_to", returnTo);
    }
    return NextResponse.redirect(start);
  }

  let session: string | undefined;
  if (isProtected) {
    session = req.cookies.get(SESSION_COOKIE)?.value;
    if (!session) {
      const login = new URL("/auth/login", req.url);
      // Path AND query. The panel keeps filters, pagination and tab selection in
      // the URL through nuqs, so a `return_to` of the bare pathname hands the user
      // back a screen they did not leave — the session expired, not the view.
      login.searchParams.set("return_to", `${req.nextUrl.pathname}${req.nextUrl.search}`);
      return NextResponse.redirect(login);
    }
  }

  const nonce = makeNonce();
  const csp = contentSecurityPolicy(nonce);

  // Forward the nonce + CSP on the request so the root layout can stamp the
  // nonce onto its <script> tags.
  const requestHeaders = new Headers(req.headers);
  requestHeaders.set("x-nonce", nonce);
  requestHeaders.set("content-security-policy", csp);

  const res = NextResponse.next({ request: { headers: requestHeaders } });
  res.headers.set("content-security-policy", csp);
  if (session) {
    res.headers.set("x-kratos-session", session);
  }
  return res;
}

export const config = {
  // Apply CSP to every document/route except static assets.
  matcher: [
    {
      source: "/((?!_next/static|_next/image|favicon.ico).*)",
    },
  ],
};
