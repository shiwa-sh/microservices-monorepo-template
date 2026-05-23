// Next.js config (ADR-0014). Standalone output is required for the Bun-only
// Dockerfile. transpilePackages exposes the in-repo TS libs to the build.
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  reactStrictMode: true,
  experimental: { typedRoutes: true },
  transpilePackages: [
    "@platform/ui",
    "@platform/server-fetch",
    "@platform/observability",
    "@platform/feature-flags",
  ],
  env: {
    NEXT_PUBLIC_SERVICE_VERSION: process.env.SERVICE_VERSION ?? "dev",
    NEXT_PUBLIC_DEPLOY_ENV: process.env.DEPLOY_ENV ?? "dev",
  },
};

export default nextConfig;
