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

// Drafting to a LEAD.
//
// A lead is the shape the person drafter's own contract describes: the record
// IS the recipient, so there is no contact to name and no deal or project to
// pick. Before this the composer refused every record but an organization, and
// "Draft with AI" on a lead did nothing at all.
//
// These assert which ENDPOINT the button reaches, because that is the whole
// question: one writer answers all three paths, and a lead reaching the account
// endpoint — or none — is the defect.

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

const LEAD_DRAFT = {
  subject: "Following up on your pricing question",
  body: "Hi Dung,\n\nYou asked what this would cost for 40 seats.",
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

describe("drafting to a lead", () => {
  it("asks the lead's own endpoint and fills the fields", async () => {
    const sent = stubRoutes({
      "POST /leads/l-1/draft-email": () => jsonResponse(LEAD_DRAFT),
    });
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@newsky.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(
      await screen.findByDisplayValue("Following up on your pricing question"),
    ).toBeTruthy();
    // The endpoint, named. A lead that reached the ACCOUNT endpoint would be
    // asking the server to ground a message in an organization it has no link
    // to, and a lead that reached none would fill nothing at all.
    expect(
      sent.some((call) => call.key === "POST /leads/l-1/draft-email"),
    ).toBe(true);
    expect(sent.some((call) => call.key.includes("/organizations/"))).toBe(
      false,
    );
  });

  it("carries the reader's own steering and nothing else", async () => {
    const sent = stubRoutes({
      "POST /leads/l-1/draft-email": () => jsonResponse(LEAD_DRAFT),
    });
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@newsky.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.type(
      screen.getByPlaceholderText(/Steer the draft/),
      "shorter",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Following up on your pricing question");

    const call = sent.find((c) => c.key === "POST /leads/l-1/draft-email");
    // The intent, and no recipient among them: naming one would be this client
    // deciding who a lead's message goes to, which is the record's own field.
    expect(call?.body).toEqual({ intent: "shorter" });
  });

  it("discloses a model-written draft", async () => {
    stubRoutes({
      "POST /leads/l-1/draft-email": () => jsonResponse(LEAD_DRAFT),
    });
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@newsky.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByTestId("ai-disclosure-banner")).toBeTruthy();
  });

  // A deployment running no model answers 501, and the composer says so rather
  // than leaving a button that does nothing when pressed.
  //
  // The ENDPOINT is asserted alongside the sentence, because the sentence alone
  // is what the composer said before this branch existed: a lead fell through
  // to "unavailable" without asking anything, so a test reading only the words
  // would pass over a tree where drafting to a lead does not work at all.
  it("says drafting is unavailable when the deployment runs no model", async () => {
    const sent = stubRoutes({
      "POST /leads/l-1/draft-email": () => jsonResponse({}, 501),
    });
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@newsky.example"
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
      sent.some((call) => call.key === "POST /leads/l-1/draft-email"),
    ).toBe(true);
  });
});
