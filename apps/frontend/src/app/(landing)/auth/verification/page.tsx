// Kratos self-service verification flow (ADR-0010, ADR-0014). Public route under
// (landing); confirms ownership of the email via a code (delivery needs a wired
// SMTP sink). The shared KratosFlow component renders the flow.
import Link from "next/link";
import { KratosFlow } from "@/components/auth/KratosFlow";
import { landing } from "@/strings/landing";

export default function VerificationPage() {
  return (
    <KratosFlow
      kind="verification"
      strings={landing.verification}
      footer={
        <Link href="/auth/login" className="mt-4 block text-sm text-brand-600 hover:underline">
          {landing.verification.toLogin}
        </Link>
      }
    />
  );
}
