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
    submit: "Sign in",
    error: "Could not start sign-in. Please retry.",
    toRegister: "Create an account",
  },
  register: {
    title: "Create account",
    starting: "Starting Kratos registration flow…",
    submit: "Create account",
    error: "Could not start registration. Please retry.",
    toLogin: "Already have an account? Sign in",
  },
  recovery: {
    title: "Recover account",
    starting: "Starting Kratos recovery flow…",
    submit: "Send recovery code",
    error: "Could not start recovery. Please retry.",
    toLogin: "Back to sign in",
  },
  settings: {
    title: "Account settings",
    starting: "Starting Kratos settings flow…",
    submit: "Save",
    error: "Could not start settings. Please retry.",
  },
} as const;
