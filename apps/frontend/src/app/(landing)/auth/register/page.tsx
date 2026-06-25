// Kratos self-service registration flow (ADR-0010, ADR-0014). Public route under
// (landing). No seeded users exist — this is how the first identity is created.
// The shared KratosFlow component renders the flow.
import Link from "next/link";
import { KratosFlow } from "@/components/auth/KratosFlow";
import { landing } from "@/strings/landing";

export default function RegisterPage() {
  return (
    <KratosFlow
      kind="registration"
      strings={landing.register}
      footer={
        <Link href="/auth/login" className="mt-4 block text-sm text-brand-600 hover:underline">
          {landing.register.toLogin}
        </Link>
      }
    />
  );
}
