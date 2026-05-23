import Link from "next/link";

export default function NotFound() {
  return (
    <main className="p-6">
      <h1 className="text-xl font-semibold">Not found</h1>
      <p className="mt-2 text-sm text-slate-600">
        That page does not exist.{" "}
        <Link href="/" className="text-brand-600 hover:underline">
          Go home
        </Link>
        .
      </p>
    </main>
  );
}
