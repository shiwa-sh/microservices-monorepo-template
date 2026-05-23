// Kratos session gate (ADR-0010, ADR-0014). (landing) is public except for
// /auth/*; the other route groups require a session. The frontend never
// validates JWTs — Tyk does that for /api/* calls. Here we only check that a
// Kratos session cookie is present and forward the session id as a header.
import { type NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "ory_kratos_session";

export function middleware(req: NextRequest) {
  const session = req.cookies.get(SESSION_COOKIE)?.value;
  if (!session) {
    const login = new URL("/auth/login", req.url);
    login.searchParams.set("return_to", req.nextUrl.pathname);
    return NextResponse.redirect(login);
  }
  const res = NextResponse.next();
  res.headers.set("x-kratos-session", session);
  return res;
}

export const config = {
  matcher: ["/panel/:path*", "/admin/:path*", "/devportal/:path*"],
};
