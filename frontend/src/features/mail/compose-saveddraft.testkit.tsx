// Shared fixtures for the saved-draft composer tests.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { writeMessage } from "../../design-system/richtext-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "../../testing/sendpermission.testkit";
import { ComposeWindow } from "./composewindow";

// The rep's own unsent message, kept by the server. The composer reads it on
// open and writes it on close, and a send discards it.

export type MailDraft = components["schemas"]["MailDraft"];
export type Sent = {
  key: string;
  query: string;
  body: unknown;
  headers: Headers;
};

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function problem(code: string, status: number) {
  return new Response(JSON.stringify({ code, title: "Conflict" }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

export const SAVED: MailDraft = {
  id: "md-1",
  anchor_type: "contact",
  anchor_id: "p-1",
  to: ["buyer@acme.test"],
  cc: [],
  bcc: [],
  subject: "Pricing for 40 seats",
  body: "Half written before lunch",
  html_body: "<p>Half written before lunch</p>",
  version: 3,
  agent_drafted: false,
  created_at: "2026-09-20T09:00:00Z",
  updated_at: "2026-09-20T09:05:00Z",
};

export const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

export type Route = () => Response | Promise<Response>;

export function stubRoutes(overrides: Record<string, Route> = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every saved-draft read and write; its branches are the route table
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET" && method !== "DELETE") {
        body = request
          ? await request.clone().json()
          : JSON.parse(String(init?.body));
      }
      const headers = request
        ? request.headers
        : new Headers(init?.headers ?? {});
      sent.push({ key, query: url.search, body, headers });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /mail-drafts") return problem("not_found", 404);
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      if (isPreviewDoor(url.pathname)) {
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({});
    }),
  );
  return sent;
}

export function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

export function composer(onClose: () => void, activityId?: string) {
  return (
    <ComposeWindow
      activityId={activityId}
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
      open
      onClose={onClose}
    />
  );
}

export const calls = (sent: Sent[], key: string) =>
  sent.filter((call) => call.key === key);

// The saved draft lands only after the reader typed, so its words never
// reached the screen.
export function foundUnderTyping(
  onClose: () => void,
  overrides: Record<string, Route> = {},
): Sent[] {
  let answer: (response: Response) => void = () => {};
  const sent = stubRoutes({
    "GET /mail-drafts": () =>
      new Promise<Response>((resolve) => {
        answer = resolve;
      }),
    ...overrides,
  });
  render(composer(onClose));
  writeMessage("Body", "Written before the read");
  act(() => answer(jsonResponse(SAVED)));
  return sent;
}
