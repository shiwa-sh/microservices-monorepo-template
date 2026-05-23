// OpenFeature wiring (ADR-0014). Day one runs the NoopProvider so calls are
// stable; the concrete backend (GrowthBook, Flipt, Unleash, …) is adopted via
// an ADR amendment on first gradual-rollout requirement.
import { OpenFeature, type Client } from "@openfeature/web-sdk";

let client: Client | undefined;

export function flagsClient(): Client {
  if (!client) {
    // NoopProvider is the default when no provider is registered.
    client = OpenFeature.getClient();
  }
  return client;
}
