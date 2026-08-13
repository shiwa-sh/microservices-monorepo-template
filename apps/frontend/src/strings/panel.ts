export const panel = {
  home: { title: "Customer panel", products: "Products", checkout: "Checkout" },
  products: { title: "Products" },
  checkout: {
    title: "Checkout",
    productPlaceholder: "product id (product_…)",
    productIdInvalid: "not a product id",
    buy: "Buy",
    starting: "starting",
    error: "error",
    running: (id: string) => `running: ${id}`,
    idle: "idle",
  },
} as const;
