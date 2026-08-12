// Kratos self-service settings flow (ADR-0304, ADR-0400). Requires a session —
// Kratos redirects the browser flow to login when unauthenticated. The shared
// KratosFlow component renders the flow.
import { KratosFlow } from "@/components/auth/KratosFlow";
import { landing } from "@/strings/landing";

export default function SettingsPage() {
  return <KratosFlow kind="settings" strings={landing.settings} />;
}
