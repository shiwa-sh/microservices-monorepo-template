"use client";

import { useEffect } from "react";
import { Button } from "@/components/base/buttons/button";
import { obsLog } from "@/lib/observability/log";

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
      <h1 className="text-xl font-semibold text-primary">Something went wrong</h1>
      <p className="mt-2 text-sm text-tertiary">{error.message}</p>
      <div className="mt-4">
        <Button onPress={reset}>Try again</Button>
      </div>
    </main>
  );
}
