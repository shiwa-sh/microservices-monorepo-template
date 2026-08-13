"use client";

import { zodResolver } from "@hookform/resolvers/zod";
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

// Catalog's own ProductId pattern (ADR-0003). A wire identifier is prefixed and
// opaque, never a bare UUID, so the field takes the form the API hands back — what a
// user pastes is what they were shown. `@libs/id` parses one where a caller needs its
// parts; a form only needs the shape the API will accept.
const productIdPattern = /^product_[0-7][0-9abcdefghjkmnpqrstvwxyz]{25}$/;

const schema = z.object({
  product_id: z.string().regex(productIdPattern, panel.checkout.productIdInvalid),
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
    const { data, error } = await orders.POST("/orders", { body: values });
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
