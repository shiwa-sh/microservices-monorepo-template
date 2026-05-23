export const devportal = {
  title: "Developer portal",
  description: "API documentation coming soon.",
  endpoints: [
    "GET /api/catalog/openapi.yaml",
    "GET /api/orders/openapi.yaml",
    "GET /api/payment/openapi.yaml",
    "GET /api/orgs/openapi.yaml",
  ] as const,
} as const;
