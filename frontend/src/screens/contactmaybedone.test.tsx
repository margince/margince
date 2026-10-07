/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ContactToday } from "./contacttoday";
import { installFetchStub, jsonResponse, meRoute } from "./story-utils";

// A promise card asking whether our last email kept it. Done is the reader's
// word for it and completes the promise; Not yet puts the question away.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-10T09:00:00Z",
  updated_at: "2026-08-10T09:00:00Z",
} as const;

const TASK = {
  id: "a-9",
  kind: "task",
  subject: "Send demo email",
  occurred_at: "2026-08-10T09:00:00Z",
  due_at: "2026-08-11T09:00:00Z",
  is_done: false,
  version: 4,
  ...CAPTURED,
} as const;

const VIEW: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
  sections_omitted: [],
  next_steps: { data: [TASK], page: { has_more: false } },
};

const ASKED = {
  promise_type: "task",
  promise_id: "a-9",
  email_activity_id: "e-1",
  wrote_at: "2026-08-12T10:00:00Z",
} as const;

const QUESTION: ContactMoment = {
  claim_key: "moment:may_be_done:a-9",
  evidence_fingerprint: "fp-1",
  rule: "overdue_promise",
  headline: "You may have done this — you wrote to them on 12 Aug",
  why_now: "You owe them: Send demo email. Due 2 days ago and still open.",
  confidence: "medium",
  evidence: [
    { type: "task", id: "a-9", label: "Send demo email" },
    { type: "activity", id: "e-1", label: "Your demo" },
  ],
  recommended_action: {
    kind: "complete_task",
    label: "Done",
    state: "available",
  },
  may_be_done: ASKED,
};

function show(moment: ContactMoment) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactToday view={VIEW} moment={moment} onAction={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a promise our last email may have kept", () => {
  it("completes the task on Done, at the version the card was drawn against", async () => {
    const patched = vi.fn();
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "PATCH /activities/a-9": (body) => {
        patched(body);
        return jsonResponse({ ...TASK, is_done: true, version: 5 });
      },
    });
    // The pinned version travels as If-Match, which the route stub does not
    // see, so the request is read on its way past.
    const routed = globalThis.fetch;
    const pinned: (string | null)[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.method === "PATCH")
          pinned.push(request.headers.get("If-Match"));
        return routed(input, init);
      }),
    );
    show(QUESTION);

    expect(screen.getByText(QUESTION.headline)).toBeTruthy();
    await userEvent.setup().click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(patched).toHaveBeenCalledWith({ is_done: true }),
    );
    expect(pinned).toEqual([String(TASK.version)]);
  });

  it("settles a claim with no task as done", async () => {
    const settled = vi.fn();
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "POST /claims/c-3/settle": (body) => {
        settled(body);
        return new Response(null, { status: 204 });
      },
    });
    show({
      ...QUESTION,
      may_be_done: { ...ASKED, promise_type: "claim", promise_id: "c-3" },
    });

    await userEvent.setup().click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(settled).toHaveBeenCalledWith({ outcome: "done" }),
    );
  });

  it("puts the question away on Not yet, against the email it showed", async () => {
    const dismissed = vi.fn();
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "POST /contacts/p-1/moment/dismiss": (body) => {
        dismissed(body);
        return new Response(null, { status: 204 });
      },
    });
    show(QUESTION);

    await userEvent
      .setup()
      .click(
        screen.getByRole("button", { name: en["contact.mayBeDone.notYet"] }),
      );
    await waitFor(() =>
      expect(dismissed).toHaveBeenCalledWith({
        claim_key: QUESTION.claim_key,
        evidence_fingerprint: QUESTION.evidence_fingerprint,
      }),
    );
  });
});
