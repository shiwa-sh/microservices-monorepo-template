# Untitled UI upstream tracking

This directory contains ports of Untitled UI primitives and design tokens. Per ADR-0014, the ports are committed source — not runtime-fetched.

| Tracked        | Version   | Synced on     |
|----------------|-----------|---------------|
| Untitled UI    | —         | not yet synced |
| lucide-react   | 0.469.0   | 2026-05-20    |

Annual bump cadence. To re-sync:

1. Pull the latest Untitled UI Tailwind preset and primitive sources.
2. Diff against the tree under `tokens/` and `components/`.
3. Open a PR titled `feat(libs/ts/ui): bump Untitled UI to <version>`.
4. Update the table above and the date.
