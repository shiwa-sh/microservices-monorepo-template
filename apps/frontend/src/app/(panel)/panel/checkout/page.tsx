"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { isId } from "@libs/id";
// Cross-service mutation (ADR-0302, ADR-0400). The orders service returns
// 202 + a workflow handle; we poll it with the shared helper instead of
// hand-rolling fetch loops.
import { useState } from "react";
import {
  Controller,
  type ControllerFieldState,
  type ControllerRenderProps,
  useForm,
} from "react-hook-form";
import { z } from "zod";
import type { BadgeColors } from "@/components/base/badges/badge-types";
import { Badge } from "@/components/base/badges/badges";
import { Button } from "@/components/base/buttons/button";
import { Input } from "@/components/base/input/input";
import { createBrowserClient } from "@/lib/server-fetch/client";
import { pollWorkflow, type WorkflowHandle } from "@/lib/server-fetch/workflow-handle";
import { panel } from "@/strings/panel";

const schema = z.object({
  // A wire identifier, not a bare UUID (ADR-0003): the field takes the form the API
  // hands back, so what a user pastes is what they were shown. `isId` is the shared
  // codec both languages check against, so the accepted shape cannot drift from the
  // one the services mint.
  product_id: z.string().refine((v) => isId(v, "product"), panel.checkout.productIdInvalid),
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

type Status = { text: string; tone: BadgeColors };

// Hoisted so the Controller render prop is a stable reference (noJsxPropsBind).
const renderProductId = ({
  field,
  fieldState,
}: {
  field: ControllerRenderProps<FormValues, "product_id">;
  fieldState: ControllerFieldState;
}) => (
  <Input
    name={field.name}
    ref={field.ref}
    value={field.value}
    onChange={field.onChange}
    onBlur={field.onBlur}
    isInvalid={fieldState.invalid}
    hint={fieldState.error?.message}
    placeholder={panel.checkout.productPlaceholder}
  />
);

export default function Checkout() {
  const [status, setStatus] = useState<Status>({ text: panel.checkout.idle, tone: "gray" });
  const orders = createBrowserClient<OrdersPaths>();

  const { control, handleSubmit, formState } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { product_id: "", quantity: 1 },
  });

  const onSubmit = handleSubmit(async (values) => {
    setStatus({ text: panel.checkout.starting, tone: "blue" });
    // One key per submit, so the browser retrying a request that already reached
    // the service gets the order it created rather than a second one (ADR-0003).
    const { data, error } = await orders.POST("/orders", {
      body: values,
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    if (error || !data) {
      setStatus({ text: panel.checkout.error, tone: "error" });
      return;
    }
    setStatus({ text: panel.checkout.running(data.id), tone: "blue" });
    try {
      // Poll the order (handle.result_url) until it reaches a terminal status; the
      // saga confirms it once catalog + payment succeed (ADR-0302).
      const order = await pollWorkflow<{ id: string; status: string }>(data);
      setStatus({
        text: order.status,
        tone: order.status === "confirmed" ? "success" : "error",
      });
    } catch {
      setStatus({ text: panel.checkout.error, tone: "error" });
    }
  });

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-2xl font-semibold">{panel.checkout.title}</h1>
      <form onSubmit={onSubmit} className="mt-4 space-y-3">
        <Controller control={control} name="product_id" render={renderProductId} />
        <Button type="submit" isLoading={formState.isSubmitting}>
          {panel.checkout.buy}
        </Button>
      </form>
      <div className="mt-3">
        <Badge type="pill-color" color={status.tone}>
          {status.text}
        </Badge>
      </div>
    </main>
  );
}
