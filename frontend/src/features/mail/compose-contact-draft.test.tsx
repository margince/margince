// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
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
import {
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { LocaleProvider } from "../../i18n";
import { draftButton } from "../../testing/composedraft.testkit";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "../../testing/sendpermission.testkit";
import { ComposeModal } from "./compose";

// Drafting to a CONTACT, from the contact's own page.
//
// The record in the path is the recipient, so the request carries nothing but
// optional steering — the shape /contacts/{id}/draft-email has described since it
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

const CONTACT_DRAFT = {
  subject: "Zwei Produkte ohne Übersetzung",
  body: "Guten Tag Frau Malherbe,\n\nfür das Beispiel brauchen wir zwei Produkte.",
  generated_by: "model",
  ai_generated: true,
  ai_disclosure: "Drafted with AI assistance. Review before sending.",
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

describe("drafting to a contact", () => {
  it("asks the contact's own endpoint and fills the fields", async () => {
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse(CONTACT_DRAFT),
    });
    render(
      <ComposeModal
        intent="Follow up on our discussion"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());

    expect(
      await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung"),
    ).toBeTruthy();
    // The endpoint, named. Reaching the ACCOUNT endpoint would ground the
    // message in whatever company sits behind the contact — a conversation the
    // rep never chose — and reaching none is what the page did before.
    expect(
      sent.some((call) => call.key === "POST /contacts/c-1/draft-email"),
    ).toBe(true);
    expect(sent.some((call) => call.key.includes("/companies/"))).toBe(false);
  });

  it("carries the reader's own steering and nothing else", async () => {
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse(CONTACT_DRAFT),
    });
    render(
      <ComposeModal
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    draftButton();
    await userEvent.type(
      screen.getByPlaceholderText(/Purpose of the email|Purpose of the reply/),
      "kurz halten",
    );
    await userEvent.click(draftButton());
    await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung");

    const call = sent.find((c) => c.key === "POST /contacts/c-1/draft-email");
    // The intent, and no recipient among them: the contact in the path IS the
    // recipient, so naming one would be this client answering a question the
    // route does not ask.
    expect(call?.body).toEqual({ intent: "kurz halten" });
  });

  // A rewrite verb has to carry the draft it is a rewrite OF. Without it the
  // server generates a second grounded draft from the same record and the
  // composer swaps it in over the first: a different email rather than a
  // shorter one, and whatever the rep had edited is gone with it.
  it("sends the draft on screen when the reader asks for a rewrite", async () => {
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse(CONTACT_DRAFT),
    });
    render(
      <ComposeModal
        intent="Follow up on our discussion"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());
    await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung");

    await userEvent.click(screen.getByRole("button", { name: "Shorter" }));

    await waitFor(() => {
      const calls = sent.filter(
        (c) => c.key === "POST /contacts/c-1/draft-email",
      );
      expect(calls.length).toBe(2);
    });
    const rewrite = sent.filter(
      (c) => c.key === "POST /contacts/c-1/draft-email",
    )[1];
    expect(rewrite.body).toEqual({
      intent: "Keep the meaning and use fewer words.",
      rewrite_of: CONTACT_DRAFT.body,
    });
  });

  // The other direction, so `rewrite_of` cannot simply be sent always: a FIRST
  // draft has nothing to rewrite, and a body sent with one would ask the model
  // to revise an empty composer.
  it("sends nothing to rewrite on a first draft", async () => {
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse(CONTACT_DRAFT),
    });
    render(
      <ComposeModal
        intent="Follow up on our discussion"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());
    await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung");

    const first = sent.find((c) => c.key === "POST /contacts/c-1/draft-email");
    expect(first?.body).toEqual({ intent: "Follow up on our discussion" });
  });

  // Words the rep typed before any draft are the rep's work. They go out with
  // no rewrite_of, and the draft is dropped with a notice.
  it("keeps the body the reader wrote and says the draft was dropped", async () => {
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse(CONTACT_DRAFT),
    });
    render(
      <ComposeModal
        intent="Make this nicer"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );
    writeMessage("Body", "hi, two products missing. pls send");

    await userEvent.click(draftButton());

    expect(
      await screen.findByText("Your edits were kept. Draft again when ready."),
    ).toBeTruthy();
    expect(messageText("Body")).toBe("hi, two products missing. pls send");
    const ask = sent.find((c) => c.key === "POST /contacts/c-1/draft-email");
    expect(ask?.body).toEqual({ intent: "Make this nicer" });
  });

  // The deterministic floor writes from the record and never reads the body.
  // Its answer must not take the place of the words the rep wrote.
  it("keeps the body the reader wrote when no model answered", async () => {
    stubRoutes({
      "POST /contacts/c-1/draft-email": () =>
        jsonResponse({
          ...CONTACT_DRAFT,
          generated_by: "deterministic",
          ai_generated: false,
          ai_disclosure: null,
        }),
    });
    render(
      <ComposeModal
        intent="Make this nicer"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );
    writeMessage("Body", "hi, two products missing. pls send");

    await userEvent.click(draftButton());

    await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung");
    expect(messageText("Body")).toContain("pls send");
  });

  it("discloses a model-written draft", async () => {
    stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse(CONTACT_DRAFT),
    });
    render(
      <ComposeModal
        intent="Follow up on our discussion"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());

    expect(
      await screen.findByRole("heading", { name: "AI-assisted draft" }),
    ).toBeTruthy();
  });

  // The sentence a deployment with no model earns, and the one this page used
  // to say without asking anybody.
  it("says the model is unconfigured only when the server says so", async () => {
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () => jsonResponse({}, 501),
    });
    render(
      <ComposeModal
        intent="Follow up on our discussion"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());

    await waitFor(() =>
      expect(screen.getByText(/AI drafting is unavailable/i)).toBeTruthy(),
    );
    expect(
      sent.some((call) => call.key === "POST /contacts/c-1/draft-email"),
    ).toBe(true);
  });

  // An origin with no route says THAT, and does not blame the deployment. A
  // deal has no draft-email endpoint to reach.
  it("says drafting is not offered here for an origin with no route", async () => {
    const sent = stubRoutes();
    render(
      <ComposeModal
        intent="Follow up on our discussion"
        entityType="deal"
        entityId="d-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());

    await waitFor(() =>
      expect(screen.getByText(/not available on this page/i)).toBeTruthy(),
    );
    // No claim about the model, because nothing was asked of it.
    expect(screen.queryByText(/model is not configured/i)).toBeNull();
    expect(sent.some((call) => call.key.includes("/draft-email"))).toBe(false);
  });
});

describe("drafting to everyone a meeting was with", () => {
  it("names the further contacts and opens with all of them in To", async () => {
    const everyone = [
      "ana@customer.example",
      "ben@customer.example",
      "cleo@customer.example",
    ];
    const sent = stubRoutes({
      "POST /contacts/c-1/draft-email": () =>
        jsonResponse({ ...CONTACT_DRAFT, to: everyone }),
    });
    render(
      <ComposeModal
        intent="follow up after the meeting"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        followUp={{ alsoTo: ["c-2", "c-3"], aboutActivityId: "m-1" }}
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(draftButton());
    await screen.findByDisplayValue("Zwei Produkte ohne Übersetzung");

    const call = sent.find((c) => c.key === "POST /contacts/c-1/draft-email");
    expect(call?.body).toMatchObject({
      also_to: ["c-2", "c-3"],
      about_activity_id: "m-1",
    });
    for (const address of everyone) {
      expect(await screen.findByText(address)).toBeTruthy();
    }
  });
});
