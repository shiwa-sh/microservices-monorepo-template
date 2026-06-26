// Shared e2e environment constants (ADR-0017 topology). Host-agnostic: override
// E2E_HOST to point the suite at a deployed env; defaults to the local edge.
import path from "node:path";

// Product apex origin (landing, /auth/*, /api/<svc>). Includes the local :8443.
export const HOST = process.env.E2E_HOST ?? "dev.localtest.me:8443";
export const BASE_URL = `https://${HOST}`;

// Operator dashboards each live on their own origin `{tool}.ops.<host>` (ADR-0017).
export const opsURL = (tool: string): string => `https://${tool}.ops.${HOST}`;

// Kratos self-service login UI (rendered by apps/frontend KratosFlow).
export const LOGIN_URL = `${BASE_URL}/auth/login`;
export const SETTINGS_URL = `${BASE_URL}/auth/settings`;

// Saved storage states produced by the `setup` project and reused by the suites.
export const AUTH_DIR = path.join(process.cwd(), ".auth");
export const OPERATOR_STATE = path.join(AUTH_DIR, "operator.json");
export const USER_STATE = path.join(AUTH_DIR, "user.json");
// The operator's enrolled TOTP secret, so the interactive login smoke can step up
// to AAL2 from a fresh context without re-enrolling.
export const OPERATOR_TOTP_FILE = path.join(AUTH_DIR, "operator-totp.txt");
