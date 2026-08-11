// Committed test-identity bootstrap (ADR-0018 §Test data). Idempotently provisions
// the deterministic identities into Kratos via the admin API and grants the
// operator group membership in OpenFGA — the same way in CI and locally, with no
// hand-seeded state and no SMTP dependency.
//
// Split of responsibility (single source of truth):
//   - cluster:full bring-up seeds the OpenFGA store + model + the static
//     dashboard->group:operator grants (platform policy; see scripts/cluster-full.sh).
//   - this bootstrap creates the Kratos identities and writes the one relation that
//     can only exist at test time: group:operator#member@user:<operator-kratos-id>.
import { IDENTITIES, OPERATOR, type TestIdentity } from "./identities";
import { portForward } from "./kube";

const KRATOS_ADMIN = "http://127.0.0.1:4434";
const SCHEMA_ID = "user_v1";
const OPENFGA_PORT = 8080;
// Local forward port for the OpenFGA HTTP API. NOT 8080: the local k3d cluster maps
// host 8080 -> the edge loadbalancer (Traefik), so binding 8080 here would collide
// with the edge and requests would hit Traefik (404) instead of OpenFGA.
const OPENFGA_LOCAL_PORT = Number(process.env.OPENFGA_LOCAL_PORT ?? 18080);
// Local/CI cluster:full preshared key (infra secret openfga-creds). Override for a
// deployed target.
const OPENFGA_TOKEN = process.env.OPENFGA_TOKEN ?? "localdevkey";

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
      // The `operator` trait is the coarse ops-gate claim (ADR-0017) and is ALWAYS
      // enforced — OpenFGA group:operator membership only feeds the optional fine
      // gate. Set it here too, or the operator fails the gate despite the grant
      // below (same coupling as scripts/ops-grant.sh).
      traits: { email: id.email, operator: id.operator },
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

const OPENFGA_API = `http://127.0.0.1:${OPENFGA_LOCAL_PORT}`;
const fgaHeaders = { authorization: `Bearer ${OPENFGA_TOKEN}`, "content-type": "application/json" };

// Discover the platform store by name (same lookup the services do).
async function storeId(): Promise<string> {
  const res = await fetch(`${OPENFGA_API}/stores`, { headers: fgaHeaders });
  if (!res.ok) {
    throw new Error(`openfga list stores failed: ${res.status} ${await res.text()}`);
  }
  const body = (await res.json()) as { stores?: { id: string; name: string }[] };
  const hit = body.stores?.find((s) => s.name === "platform");
  if (!hit) {
    throw new Error("openfga store 'platform' not found — has the seed Job run?");
  }
  return hit.id;
}

// Write a tuple, tolerating the idempotent "already existed" duplicate error.
async function writeTuple(sid: string, user: string, relation: string, object: string): Promise<void> {
  const res = await fetch(`${OPENFGA_API}/stores/${sid}/write`, {
    method: "POST",
    headers: fgaHeaders,
    body: JSON.stringify({ writes: { tuple_keys: [{ user, relation, object }] } }),
  });
  if (res.ok) {
    return;
  }
  const text = await res.text();
  if (!text.includes("already existed")) {
    throw new Error(`openfga write failed: ${res.status} ${text}`);
  }
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
  // is `user:<kratos-id>`). The write is idempotent.
  const openfgaPf = await portForward("openfga", OPENFGA_LOCAL_PORT, OPENFGA_PORT);
  try {
    const sid = await storeId();
    await writeTuple(sid, `user:${ids[OPERATOR.label]}`, "member", "group:operator");
  } finally {
    openfgaPf.stop();
  }

  return ids;
}
