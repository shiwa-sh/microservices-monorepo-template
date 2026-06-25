// Kratos self-service recovery flow (ADR-0010, ADR-0014). Public route under
// (landing); emails a recovery code/link (delivery needs a wired SMTP sink). The
// shared KratosFlow component renders the flow.
import Link from "next/link";
import { KratosFlow } from "@/components/auth/KratosFlow";
import { landing } from "@/strings/landing";

export default function RecoveryPage() {
  return (
    <KratosFlow
      kind="recovery"
      strings={landing.recovery}
      footer={
        <Link href="/auth/login" className="mt-4 block text-sm text-brand-600 hover:underline">
          {landing.recovery.toLogin}
        </Link>
      }
    />
  );
}
