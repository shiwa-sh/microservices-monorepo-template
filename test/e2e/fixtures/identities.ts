// Committed deterministic test identities (ADR-0601). Provisioned the same way in
// CI and locally — nothing is hand-seeded. `operator` mirrors OpenFGA `user:alice`
// (an AAL2 operator in group:operator); `user` mirrors `user:bob` (a bare AAL1
// product user with no ops grant). See infra/auth/openfga/fga.yaml.
//
// `reset` splits the lifecycle: the identities the e2e suite logs in as are
// recreated per run for determinism (fresh password-only identity => the same
// login -> enrol -> AAL2 path). `admin` is the human's day-to-day full-tier
// account, so it is created-if-missing and left alone once it exists — an e2e run
// must not wipe a developer's session and TOTP.
export type TestIdentity = {
  label: "operator" | "user" | "admin";
  email: string;
  password: string;
  // operator => enrolled in TOTP (AAL2) and added to group:operator at bootstrap.
  operator: boolean;
  // true => deleted and recreated per e2e run; false => created once, then kept.
  reset: boolean;
};

export const OPERATOR: TestIdentity = {
  label: "operator",
  email: "operator@e2e.localtest.me",
  password: "0perator-e2e-Sessi0n!",
  operator: true,
  reset: true,
};

export const ADMIN: TestIdentity = {
  label: "admin",
  email: "admin@localtest.me",
  password: "1st Password!",
  operator: true,
  reset: false,
};

export const USER: TestIdentity = {
  label: "user",
  email: "user@e2e.localtest.me",
  password: "Pr0duct-e2e-Sessi0n!",
  operator: false,
  reset: true,
};

export const IDENTITIES: TestIdentity[] = [OPERATOR, ADMIN, USER];
