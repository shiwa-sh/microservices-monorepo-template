// Shared Kratos self-service flow renderer (ADR-0010, ADR-0014). Every /auth/*
// page is the same shape: fetch the flow from the Kratos public API (same origin
// via Traefik, /auth/self-service/<flow>/* → ory-kratos-public) and render its UI
// nodes as a native form that POSTs straight back to Kratos — no client SDK; the
// session and CSRF cookies are Kratos's. Client component because the flow id and
// those cookies only exist in the browser.
"use client";

import type { ReactNode } from "react";
import { useEffect, useState } from "react";

export type FlowKind = "login" | "registration" | "recovery" | "settings";

export type FlowStrings = {
  title: string;
  starting: string;
  submit: string;
  error: string;
};

type UiText = { id: number; text: string };
type UiNode = {
  attributes: {
    node_type?: string;
    name?: string;
    type?: string;
    value?: string | number | boolean;
    required?: boolean;
    disabled?: boolean;
  };
  messages?: UiText[];
  meta: { label?: UiText };
};
type Flow = {
  ui: { action: string; method: string; nodes: UiNode[]; messages?: UiText[] };
};

// Kratos sets the CSRF cookie and redirects back here with ?flow=<id>; (re)start
// the browser flow, preserving any return_to.
function restartFlow(kind: FlowKind): void {
  const returnTo = new URLSearchParams(window.location.search).get("return_to");
  const init = new URL(`/auth/self-service/${kind}/browser`, window.location.origin);
  if (returnTo) {
    init.searchParams.set("return_to", returnTo);
  }
  window.location.replace(init.toString());
}

function FlowField({ node, submitLabel }: { node: UiNode; submitLabel: string }) {
  const attr = node.attributes;
  const value = String(attr.value ?? "");
  const labelText = node.meta.label ? node.meta.label.text : undefined;

  if (attr.type === "hidden") {
    return <input type="hidden" name={attr.name} value={value} />;
  }
  if (attr.type === "submit") {
    return (
      <button
        type="submit"
        name={attr.name}
        value={value}
        className="w-full rounded bg-brand-600 px-4 py-2 text-white hover:bg-brand-700"
      >
        {labelText ?? submitLabel}
      </button>
    );
  }
  return (
    <label className="block">
      <span className="text-sm text-slate-600">{labelText ?? attr.name}</span>
      <input
        name={attr.name}
        type={attr.type}
        required={attr.required}
        disabled={attr.disabled}
        defaultValue={attr.type === "password" ? undefined : value}
        className="mt-1 w-full rounded border border-slate-300 px-3 py-2"
      />
      {node.messages?.map((message) => (
        <span key={message.id} className="mt-1 block text-sm text-red-600">
          {message.text}
        </span>
      ))}
    </label>
  );
}

export function KratosFlow({
  kind,
  strings,
  footer,
}: {
  kind: FlowKind;
  strings: FlowStrings;
  footer?: ReactNode;
}) {
  const [flow, setFlow] = useState<Flow | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("flow");
    if (!id) {
      restartFlow(kind);
      return;
    }
    fetch(`/auth/self-service/${kind}/flows?id=${encodeURIComponent(id)}`, {
      headers: { accept: "application/json" },
      credentials: "include",
    })
      .then((res) => {
        if (res.status === 404 || res.status === 410) {
          // Expired or unknown flow — start a fresh one.
          restartFlow(kind);
          return null;
        }
        if (!res.ok) {
          throw new Error(String(res.status));
        }
        return res.json() as Promise<Flow>;
      })
      .then((data) => data && setFlow(data))
      .catch(() => setFailed(true));
  }, [kind]);

  if (failed) {
    return (
      <main className="mx-auto max-w-md p-6">
        <h1 className="text-2xl font-semibold">{strings.title}</h1>
        <p className="mt-2 text-red-600">{strings.error}</p>
      </main>
    );
  }

  if (!flow) {
    return (
      <main className="mx-auto max-w-md p-6">
        <h1 className="text-2xl font-semibold">{strings.title}</h1>
        <p className="mt-2 text-slate-600">{strings.starting}</p>
      </main>
    );
  }

  const inputs = flow.ui.nodes.filter((node) => node.attributes.node_type === "input");

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-2xl font-semibold">{strings.title}</h1>
      {flow.ui.messages?.map((message) => (
        <p key={message.id} className="mt-2 text-slate-600">
          {message.text}
        </p>
      ))}
      <form method={flow.ui.method} action={flow.ui.action} className="mt-4 space-y-3">
        {inputs.map((node) => (
          <FlowField key={node.attributes.name} node={node} submitLabel={strings.submit} />
        ))}
      </form>
      {footer}
    </main>
  );
}
