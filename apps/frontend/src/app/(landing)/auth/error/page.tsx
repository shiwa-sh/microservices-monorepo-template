// Kratos self-service error flow (ADR-0304, ADR-0400). Public route under
// (landing); the shared AuthError component renders the error detail.
import { AuthError } from "@/components/auth/AuthError";

export default function AuthErrorPage() {
  return <AuthError />;
}
