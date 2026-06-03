"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { createBrowserClient, pollWorkflow, type WorkflowHandle } from "@server-fetch";
import { Button } from "@ui";
import { panel } from "@ui/strings/panel";
// Cross-service mutation (ADR-0006, ADR-0014). The orders service returns
// 202 + a workflow handle; we poll it with the shared helper instead of
// hand-rolling fetch loops.
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

const schema = z.object({
  product_id: z.string().uuid(),
  quantity: z.number().int().positive(),
});

type FormValues = z.infer<typeof schema>;

type OrdersPaths = {
  "/orders": {
    post: {
      requestBody: { content: { "application/json": FormValues } };
      responses: { 202: { content: { "application/json": WorkflowHandle } } };
    };
  };
};

export default function Checkout() {
  const [status, setStatus] = useState(panel.checkout.idle);
  const orders = createBrowserClient<OrdersPaths>("orders");

  const { register, handleSubmit, formState } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { product_id: "", quantity: 1 },
  });

  const onSubmit = handleSubmit(async (values) => {
    setStatus(panel.checkout.starting);
    const { data, error } = await orders.POST("/orders", { body: values });
    if (error || !data) {
      setStatus(panel.checkout.error);
      return;
    }
    setStatus(panel.checkout.running(data.id));
    const result = await pollWorkflow(data);
    setStatus(result.status);
  });

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-2xl font-semibold">{panel.checkout.title}</h1>
      <form onSubmit={onSubmit} className="mt-4 space-y-3">
        <input
          {...register("product_id")}
          placeholder={panel.checkout.productPlaceholder}
          className="w-full rounded-md border border-slate-300 px-3 py-2"
        />
        <Button type="submit" disabled={formState.isSubmitting}>
          {panel.checkout.buy}
        </Button>
      </form>
      <p className="mt-3 text-sm text-slate-600">{status}</p>
    </main>
  );
}
