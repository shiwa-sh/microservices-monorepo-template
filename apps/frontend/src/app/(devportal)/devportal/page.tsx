// Developer portal (ADR-0009). Placeholder until the first external partner.
import { devportal } from "@ui/strings/devportal";

export default function DevPortal() {
  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-2xl font-semibold">{devportal.title}</h1>
      <p className="mt-2 text-slate-600">{devportal.description}</p>
      <ul className="mt-4 space-y-1 font-mono text-sm">
        {devportal.endpoints.map((e) => (
          <li key={e}>{e}</li>
        ))}
      </ul>
    </main>
  );
}
