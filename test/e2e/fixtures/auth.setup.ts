// The Playwright `setup` project (ADR-0018): provision the committed identities,
// then log each one in through the real UI and save its session as storage state
// for the suites to reuse. Runs before `platform` (see playwright.config.ts).
import fs from "node:fs";
import { test as setup } from "@playwright/test";
import { provision } from "./bootstrap";
import { AUTH_DIR, OPERATOR_STATE, OPERATOR_TOTP_FILE, USER_STATE } from "./env";
import { OPERATOR, USER } from "./identities";
import { ensureAal2, enrolTotp, passwordLogin } from "./kratos";

setup.describe.configure({ mode: "serial" });

setup("provision committed identities", async () => {
  fs.mkdirSync(AUTH_DIR, { recursive: true });
  await provision();
});

setup("operator AAL2 session", async ({ page, context }) => {
  await passwordLogin(page, OPERATOR.email, OPERATOR.password);
  const secret = await enrolTotp(page);
  await ensureAal2(page, secret);
  fs.writeFileSync(OPERATOR_TOTP_FILE, secret);
  await context.storageState({ path: OPERATOR_STATE });
});

setup("user AAL1 session", async ({ page, context }) => {
  await passwordLogin(page, USER.email, USER.password);
  await context.storageState({ path: USER_STATE });
});
