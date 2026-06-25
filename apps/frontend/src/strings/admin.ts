// The Lowdefy internal admin console now lives on its own ops origin
// (console.ops.<host>, ADR-0017), not a product path. Host-configurable.
const consoleUrl =
  process.env.NEXT_PUBLIC_CONSOLE_URL ?? "https://console.ops.dev.localtest.me:8443/";

export const admin = {
  title: "Internal admin",
  description: "Open the admin console.",
  href: consoleUrl,
  forbidden: "You do not have access to the internal admin.",
} as const;
