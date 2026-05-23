import Link from "next/link";
import { landing } from "@ui/strings/landing";

export default function Landing() {
  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-3xl font-semibold">{landing.title}</h1>
      <p className="mt-2 text-slate-600">{landing.tagline}</p>
      <ul className="mt-6 space-y-2">
        <li>
          <Link href="/auth/login" className="text-brand-600 hover:underline">
            {landing.signIn}
          </Link>
        </li>
        <li>
          <Link href="/panel" className="text-brand-600 hover:underline">
            {landing.customerPanel}
          </Link>
        </li>
        <li>
          <Link href="/devportal" className="text-brand-600 hover:underline">
            {landing.developerPortal}
          </Link>
        </li>
      </ul>
    </main>
  );
}
