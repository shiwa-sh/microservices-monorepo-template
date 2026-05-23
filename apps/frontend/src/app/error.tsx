"use client";

import { useEffect } from "react";
import { Button } from "@ui";
import { obsLog } from "@observability/client";

export default function RootError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    obsLog.error(error, { digest: error.digest });
  }, [error]);

  return (
    <main className="p-6">
      <h1 className="text-xl font-semibold">Something went wrong</h1>
      <p className="mt-2 text-sm text-slate-600">{error.message}</p>
      <Button className="mt-4" onClick={reset}>
        Try again
      </Button>
    </main>
  );
}
