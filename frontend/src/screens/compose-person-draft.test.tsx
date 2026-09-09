// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

// Drafting to a PERSON, from the contact's own page.
//
// The record in the path is the recipient, so the request carries nothing but
// optional steering — the shape /people/{id}/draft-email has described since it
// shipped, answered by the same writer as the account and lead paths.
//
// The composer never called it. "Write email" on a contact fell through to the
// company guard, and the rep was told "the model is not configured" — a
// claim about the deployment, made by a browser that had sent no request, on a
// stack whose model was answering transcript readings on the same screen. So
// these assert the ENDPOINT and not merely the words: a test reading the
// sentence alone passes on the tree where this feature does not work.

type Sent = { key: string; body: unknown };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const PURPOSES = {
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

const PERSON_DRAFT = {
  subject: "Zwei Produkte ohne Übersetzung",
  body: "Guten Tag Frau Malherbe,\n\nfür das Beispiel brauchen wir zwei Produkte.",
  generated_by: "model",
  ai_generated: true,
  ai_disclosure: "This message was drafted with AI assistance.",
};

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, body });
      const override = overrides[key];
      if (override) return override();
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

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("drafting to a person", () => {
  it("asks the person's own endpoint and fills the fields", async () => {
    const sent = stubRoutes({
      "POST /people/c-1/draft-email": () => jsonResponse(PERSON_DRAFT),
    });
    render(
      <ComposeModal
        entityType="person"
        entityId="c-1"
        personId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(
      await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung"),
    ).toBeTruthy();
    // The endpoint, named. Reaching the ACCOUNT endpoint would ground the
    // message in whatever company sits behind the contact — a conversation the
    // rep never chose — and reaching none is what the page did before.
    expect(
      sent.some((call) => call.key === "POST /people/c-1/draft-email"),
    ).toBe(true);
    expect(sent.some((call) => call.key.includes("/companies/"))).toBe(
      false,
    );
  });

  it("carries the reader's own steering and nothing else", async () => {
    const sent = stubRoutes({
      "POST /people/c-1/draft-email": () => jsonResponse(PERSON_DRAFT),
    });
    render(
      <ComposeModal
        entityType="person"
        entityId="c-1"
        personId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.type(
      screen.getByPlaceholderText(/Steer the draft/),
      "kurz halten",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung");

    const call = sent.find((c) => c.key === "POST /people/c-1/draft-email");
    // The intent, and no recipient among them: the person in the path IS the
    // recipient, so naming one would be this client answering a question the
    // route does not ask.
    expect(call?.body).toEqual({ intent: "kurz halten" });
  });

  it("discloses a model-written draft", async () => {
    stubRoutes({
      "POST /people/c-1/draft-email": () => jsonResponse(PERSON_DRAFT),
    });
    render(
      <ComposeModal
        entityType="person"
        entityId="c-1"
        personId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(
      await screen.findByRole("heading", { name: "AI-assisted draft" }),
    ).toBeTruthy();
  });

  // The sentence a deployment with no model earns, and the one this page used
  // to say without asking anybody.
  it("says the model is unconfigured only when the server says so", async () => {
    const sent = stubRoutes({
      "POST /people/c-1/draft-email": () => jsonResponse({}, 501),
    });
    render(
      <ComposeModal
        entityType="person"
        entityId="c-1"
        personId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/AI drafting is unavailable/i)).toBeTruthy(),
    );
    expect(
      sent.some((call) => call.key === "POST /people/c-1/draft-email"),
    ).toBe(true);
  });

  // An origin with no route says THAT, and does not blame the deployment. A
  // deal has no draft-email endpoint to reach.
  it("says drafting is not offered here for an origin with no route", async () => {
    const sent = stubRoutes();
    render(
      <ComposeModal
        entityType="deal"
        entityId="d-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/not offered from this page/i)).toBeTruthy(),
    );
    // No claim about the model, because nothing was asked of it.
    expect(screen.queryByText(/model is not configured/i)).toBeNull();
    expect(sent.some((call) => call.key.includes("/draft-email"))).toBe(false);
  });
});
