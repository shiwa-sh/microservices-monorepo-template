// Shared Kratos self-service flow renderer (ADR-0304, ADR-0400). Every /auth/*
// page is the same shape: fetch the flow from the Kratos public API (same origin
// via Traefik, /auth/self-service/<flow>/* → ory-kratos-public) and render its UI
// nodes as a native form that POSTs straight back to Kratos — no client SDK; the
// session and CSRF cookies are Kratos's. Client component because the flow id and
// those cookies only exist in the browser.
"use client";

import type { HTMLAttributeReferrerPolicy, ReactNode } from "react";
import { useEffect, useState } from "react";
import { Input } from "@/components/base/input/input";

export type FlowKind = "login" | "registration" | "recovery" | "verification" | "settings";

export type FlowStrings = {
  title: string;
  starting: string;
  submit: string;
  error: string;
};

type UiText = { id: number; text: string };
type UiNode = {
  type?: string; // node_type at the node level: input | text | img | script | a
  group?: string;
  attributes: {
    node_type?: string;
    name?: string;
    type?: string;
    value?: string | number | boolean;
    required?: boolean;
    disabled?: boolean;
    // text node (e.g. the TOTP secret) and img node (the TOTP QR code)
    text?: UiText;
    src?: string;
    // webauthn registration is a submit button whose handler the script node sets
    onclick?: string;
    // script node attributes (Kratos WebAuthn helper)
    id?: string;
    async?: boolean;
    referrerpolicy?: string;
    crossorigin?: string;
    integrity?: string;
    nonce?: string;
  };
  messages?: UiText[];
  meta: { label?: UiText };
};

function nodeType(node: UiNode): string {
  return node.type ?? node.attributes.node_type ?? "input";
}

function nodeKey(node: UiNode): string {
  const attr = node.attributes;
  return [nodeType(node), attr.name, attr.type, attr.value, attr.id, attr.text?.id, attr.src].join(
    ":",
  );
}
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

// Kratos WebAuthn helper script. Same-origin src; the nonce lets it run under the
// strict CSP (strict-dynamic). It defines window.__oryWebAuthn* used by buttons.
function ScriptNode({ attr }: { attr: UiNode["attributes"] }) {
  return (
    <script
      src={attr.src}
      async={attr.async}
      nonce={attr.nonce}
      crossOrigin={attr.crossorigin as "anonymous" | "use-credentials" | undefined}
      integrity={attr.integrity}
      referrerPolicy={attr.referrerpolicy as HTMLAttributeReferrerPolicy | undefined}
    />
  );
}

// TOTP QR code. A plain <img> is correct: Kratos returns a data: URL (allowed by
// img-src 'self' data:), not a routable next/image asset.
function ImgNode({ src, alt }: { src?: string; alt: string }) {
  // biome-ignore lint/performance/noImgElement: data: URL QR, not a next/image asset.
  return <img src={src} alt={alt} width={200} height={200} className="my-2" />;
}

function InputNode({ node, submitLabel }: { node: UiNode; submitLabel: string }) {
  const attr = node.attributes;
  const value = String(attr.value ?? "");
  const labelText = node.meta.label ? node.meta.label.text : undefined;

  if (attr.type === "hidden") {
    return <input type="hidden" name={attr.name} value={value} />;
  }
  if (attr.type === "submit" || attr.type === "button") {
    // Stays a native button (not the Untitled <Button>): Kratos identifies the
    // pressed method by this button's name=value, and a settings flow renders
    // every method (password, WebAuthn, TOTP) in one form, each submit needing
    // `formNoValidate` so one method's empty field can't block another's submit.
    // React Aria's Button strips name/value/formNoValidate, so it can't be used
    // here. Kratos validates the submitted method server-side.
    return (
      <button
        type={attr.type === "button" ? "button" : "submit"}
        name={attr.name}
        value={value}
        formNoValidate
        className="w-full rounded bg-brand-600 px-4 py-2 text-white hover:bg-brand-700"
      >
        {labelText ?? submitLabel}
      </button>
    );
  }
  const messages = node.messages ?? [];
  const errorText = messages.map((message) => message.text).join(" ");
  return (
    <Input
      label={labelText ?? attr.name}
      name={attr.name}
      // Kratos drives the input type (email, password, text, …).
      type={attr.type as "text" | "email" | "password" | "search" | "tel" | "url"}
      isRequired={attr.required}
      isDisabled={attr.disabled}
      isInvalid={messages.length > 0}
      hint={errorText || undefined}
      // Never pre-fill password fields from the flow response.
      defaultValue={attr.type === "password" ? undefined : value}
    />
  );
}

// Renders a single Kratos UI node. Beyond inputs, settings flows for MFA emit
// `text` (the TOTP secret), `img` (the TOTP QR code) and `script` (the WebAuthn
// helper) nodes, so an operator can enrol a second factor (AAL2, ADR-0304).
function FlowNode({ node, submitLabel }: { node: UiNode; submitLabel: string }) {
  const attr = node.attributes;
  const labelText = node.meta.label ? node.meta.label.text : undefined;
  switch (nodeType(node)) {
    case "text":
      // e.g. the TOTP shared secret to type into an authenticator app.
      return (
        <p className="break-all rounded bg-secondary p-2 font-mono text-sm text-primary">
          {attr.text?.text}
        </p>
      );
    case "img":
      return <ImgNode src={attr.src} alt={labelText ?? "QR code"} />;
    case "script":
      return <ScriptNode attr={attr} />;
    default:
      return <InputNode node={node} submitLabel={submitLabel} />;
  }
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
        <p className="mt-2 text-tertiary">{strings.starting}</p>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="text-2xl font-semibold">{strings.title}</h1>
      {flow.ui.messages?.map((message) => (
        <p key={message.id} className="mt-2 text-tertiary">
          {message.text}
        </p>
      ))}
      <form method={flow.ui.method} action={flow.ui.action} className="mt-4 space-y-3">
        {flow.ui.nodes.map((node) => (
          <FlowNode key={nodeKey(node)} node={node} submitLabel={strings.submit} />
        ))}
      </form>
      {footer}
    </main>
  );
}
