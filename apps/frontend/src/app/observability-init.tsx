"use client";

import { useEffect } from "react";
import { initBrowserObservability } from "@observability/client";

export function ObservabilityInit() {
  useEffect(() => {
    initBrowserObservability();
  }, []);
  return null;
}
