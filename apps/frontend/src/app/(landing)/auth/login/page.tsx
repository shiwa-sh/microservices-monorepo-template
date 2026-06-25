// Kratos self-service login flow (ADR-0010, ADR-0014). Public route under
// (landing); the shared KratosFlow component renders the flow.
import Link from "next/link";
import { KratosFlow } from "@/components/auth/KratosFlow";
import { landing } from "@/strings/landing";

export default function LoginPage() {
  return (
    <KratosFlow
      kind="login"
      strings={landing.auth}
      footer={
        <Link href="/auth/register" className="mt-4 block text-sm text-brand-600 hover:underline">
          {landing.auth.toRegister}
        </Link>
      }
    />
  );
}
