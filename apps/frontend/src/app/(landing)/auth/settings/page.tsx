// Kratos self-service settings flow (ADR-0010, ADR-0014). Requires a session —
// Kratos redirects the browser flow to login when unauthenticated. The shared
// KratosFlow component renders the flow.
import { KratosFlow } from "@/components/auth/KratosFlow";
import { landing } from "@/strings/landing";

export default function SettingsPage() {
  return <KratosFlow kind="settings" strings={landing.settings} />;
}
