// Committed deterministic test identities (ADR-0018). Provisioned the same way in
// CI and locally — nothing is hand-seeded. `operator` mirrors SpiceDB `user:alice`
// (an AAL2 operator in group:operator); `user` mirrors `user:bob` (a bare AAL1
// product user with no ops grant). See infra/auth/spicedb/*.yaml.
export type TestIdentity = {
  label: "operator" | "user";
  email: string;
  password: string;
  // operator => enrolled in TOTP (AAL2) and added to group:operator at bootstrap.
  operator: boolean;
};

export const OPERATOR: TestIdentity = {
  label: "operator",
  email: "operator@e2e.localtest.me",
  password: "0perator-e2e-Sessi0n!",
  operator: true,
};

export const USER: TestIdentity = {
  label: "user",
  email: "user@e2e.localtest.me",
  password: "Pr0duct-e2e-Sessi0n!",
  operator: false,
};

export const IDENTITIES: TestIdentity[] = [OPERATOR, USER];
