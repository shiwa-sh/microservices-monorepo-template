// User-facing strings for the (landing) route group (ADR-0014). One file per
// route group; migration to next-intl is mechanical when a non-English locale
// lands on the roadmap.
export const landing = {
  title: "Platform",
  tagline: "Self-hosted microservices template.",
  signIn: "Sign in",
  customerPanel: "Customer panel",
  developerPortal: "Developer portal",
  auth: {
    title: "Sign in",
    starting: "Starting Kratos login flow…",
    begin: "Begin",
    flowLabel: "Flow",
  },
} as const;
