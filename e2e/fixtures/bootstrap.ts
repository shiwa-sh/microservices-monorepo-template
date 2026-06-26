// Committed test-identity bootstrap (ADR-0018 §Test data). Idempotently provisions
// the deterministic identities into Kratos via the admin API and grants the
// operator group membership in SpiceDB — the same way in CI and locally, with no
// hand-seeded state and no SMTP dependency.
//
// Split of responsibility (single source of truth):
//   - cluster:full bring-up seeds the SpiceDB schema + the static
//     dashboard->group:operator grants (platform policy; see scripts/cluster-full.sh).
//   - this bootstrap creates the Kratos identities and writes the one relation that
//     can only exist at test time: group:operator#member@user:<operator-kratos-id>.
import { execFileSync } from "node:child_process";
import { IDENTITIES, OPERATOR, type TestIdentity } from "./identities";
import { portForward } from "./kube";

const KRATOS_ADMIN = "http://127.0.0.1:4434";
const SCHEMA_ID = "user_v1";
const SPICEDB_PORT = 50051;
// Local/CI cluster:full preshared key (infra secret spicedb-creds). Override for a
// deployed target.
const SPICEDB_TOKEN = process.env.SPICEDB_TOKEN ?? "localdevkey";

type KratosIdentity = { id: string; traits: { email: string } };

async function findIdentity(email: string): Promise<string | null> {
  const res = await fetch(
    `${KRATOS_ADMIN}/admin/identities?credentials_identifier=${encodeURIComponent(email)}`,
  );
  if (!res.ok) {
    throw new Error(`kratos admin list failed: ${res.status} ${await res.text()}`);
  }
  const list = (await res.json()) as KratosIdentity[];
  const hit = list.find((i) => i.traits?.email === email);
  return hit?.id ?? null;
}

async function deleteIdentity(id: string): Promise<void> {
  const res = await fetch(`${KRATOS_ADMIN}/admin/identities/${id}`, { method: "DELETE" });
  if (!res.ok && res.status !== 404) {
    throw new Error(`kratos admin delete failed for ${id}: ${res.status} ${await res.text()}`);
  }
}

async function createIdentity(id: TestIdentity): Promise<string> {
  const res = await fetch(`${KRATOS_ADMIN}/admin/identities`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      schema_id: SCHEMA_ID,
      traits: { email: id.email },
      // Import path: the password is hashed by Kratos and is NOT run through the
      // sign-up policy (HIBP/length) — deterministic committed creds are fine.
      credentials: { password: { config: { password: id.password } } },
      // Pre-verify the address so login is never gated on the (unwired) SMTP sink.
      verifiable_addresses: [
        { value: id.email, via: "email", verified: true, status: "completed" },
      ],
    }),
  });
  if (!res.ok) {
    throw new Error(`kratos admin create failed for ${id.email}: ${res.status} ${await res.text()}`);
  }
  return ((await res.json()) as KratosIdentity).id;
}

// resetIdentity recreates the identity from scratch. Kratos cannot import a TOTP
// credential (admin create rejects it), so the operator's second factor is enrolled
// at runtime via the settings flow — which requires a known starting state. Deleting
// any prior identity makes every run deterministic (fresh password-only identity =>
// the same login -> enrol -> AAL2 path), instead of state-dependent two-factor login.
async function resetIdentity(id: TestIdentity): Promise<string> {
  const existing = await findIdentity(id.email);
  if (existing) {
    await deleteIdentity(existing);
  }
  return createIdentity(id);
}

function zed(args: string[]): void {
  execFileSync(
    "zed",
    [...args, "--endpoint", `127.0.0.1:${SPICEDB_PORT}`, "--insecure", "--token", SPICEDB_TOKEN],
    { stdio: "pipe" },
  );
}

// provision creates both identities and writes the operator's group membership.
// Returns the Kratos id of each, so the setup project can correlate sessions.
export async function provision(): Promise<Record<string, string>> {
  const kratosPf = await portForward("ory-kratos-admin", 4434, 80);
  const ids: Record<string, string> = {};
  try {
    for (const id of IDENTITIES) {
      ids[id.label] = await resetIdentity(id);
    }
  } finally {
    kratosPf.stop();
  }

  // Operator membership keyed by the freshly-created Kratos id (the authz subject
  // is `user:<kratos-id>`). `touch` is idempotent.
  const spicedbPf = await portForward("spicedb", SPICEDB_PORT, SPICEDB_PORT);
  try {
    zed(["relationship", "touch", "group:operator", "member", `user:${ids[OPERATOR.label]}`]);
  } finally {
    spicedbPf.stop();
  }

  return ids;
}
