// Root loading boundary (ADR-0400). Per-route-group versions override this.
import { LoadingIndicator } from "@/components/application/loading-indicator/loading-indicator";

export default function Loading() {
  return (
    <main className="p-6" aria-busy="true">
      <LoadingIndicator size="md" label="Loading…" />
    </main>
  );
}
