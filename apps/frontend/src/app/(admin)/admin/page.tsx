// Admin tile (ADR-0012, ADR-0017). The real admin lives on its own ops origin
// (console.ops.<host>). Page-level access is a permission check in this RSC layer
// (ADR-0017) — proxy.ts only proves a session exists; here we require the `admin`
// role, so a bare logged-in user is denied.
import { hasRole, whoami } from "@/lib/auth/session";
import { admin } from "@/strings/admin";

export default async function AdminTile() {
  const session = await whoami();
  if (!hasRole(session, "admin")) {
    return (
      <main className="mx-auto max-w-3xl p-6">
        <h1 className="text-2xl font-semibold">Forbidden</h1>
        <p className="mt-2 text-slate-600">{admin.forbidden}</p>
      </main>
    );
  }
  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="text-2xl font-semibold">{admin.title}</h1>
      <p className="mt-2 text-slate-600">
        Open the admin console at{" "}
        <a href={admin.href} className="text-brand-600 hover:underline">
          {admin.href}
        </a>
        .
      </p>
    </main>
  );
}
