// Kratos self-service login flow (ADR-0010, ADR-0014). Public route under
// (landing); the Kratos browser endpoint shares the host via Traefik.
import { landing } from "@ui/strings/landing";

type Search = Promise<{ flow?: string }>;

export default async function LoginPage({ searchParams }: { searchParams: Search }) {
  const { flow } = await searchParams;

  if (!flow) {
    return (
      <main className="mx-auto max-w-md p-6">
        <h1 className="text-2xl font-semibold">{landing.auth.title}</h1>
        <p className="mt-2 text-slate-600">{landing.auth.starting}</p>
        <a
          href="/auth/self-service/login/browser"
          className="mt-4 inline-block text-brand-600 hover:underline"
        >
          {landing.auth.begin}
        </a>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-2xl font-semibold">{landing.auth.title}</h1>
      <p className="mt-2 text-slate-600">
        {landing.auth.flowLabel}: <code>{flow}</code>
      </p>
      {/* Render the Kratos flow's UI nodes here. */}
    </main>
  );
}
