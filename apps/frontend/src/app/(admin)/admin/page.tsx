// Admin tile (ADR-0012). The real admin lives at /internal/admin (Lowdefy).
import { admin } from "@ui/strings/admin";

export default function AdminTile() {
  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-2xl font-semibold">{admin.title}</h1>
      <p className="mt-2 text-slate-600">
        Open the admin tool at{" "}
        <a href={admin.href} className="text-brand-600 hover:underline">
          {admin.href}
        </a>
        .
      </p>
    </main>
  );
}
