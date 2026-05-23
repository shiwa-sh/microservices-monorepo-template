// Server-side logger (ADR-0011, ADR-0014). Structured JSON to stdout via pino,
// enriched with the active span's trace_id when available.
import "server-only";

import pino from "pino";

export const log = pino({
  level: process.env.LOG_LEVEL ?? "info",
  base: {
    service: "frontend",
    version: process.env.SERVICE_VERSION ?? "dev",
  },
  // Render JSON; no pretty-printing in prod. stdout-first per ADR-0011.
  formatters: {
    level: (label) => ({ level: label }),
  },
});
