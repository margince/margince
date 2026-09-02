// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordEmailAside } from "./recordemail";

// The box offers one of two things and never nothing, and WHICH one it offers
// is the whole feature: a `replyTo` means an answer is owed, its absence means
// the record has nothing to answer. A caller that already knows its reply
// target (dealemail.tsx) passes `replyTo` directly, so the first tests below
// drive it through the prop the same way. `detectWaitingReply` is the other
// path — a person or lead page with no status card of its own — and is
// covered separately further down, mocked at the fetch boundary the way
// compose.test.tsx mocks its own served activities.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const PERSON = "01a03000-0000-7000-8000-000000000002";
const MAIL = "01a03000-0000-7000-8000-0000000000bb";

function renderBox(replyTo?: string) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <RecordEmailAside
          entityType="person"
          entityId={PERSON}
          replyTo={replyTo}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the record's email box", () => {
  it("offers the reply when the caller passes a reply target", () => {
    renderBox(MAIL);
    expect(
      screen.getByRole("button", { name: "Draft the reply" }),
    ).toBeTruthy();
  });

  it("offers a fresh mail when there is no reply target", () => {
    renderBox();
    expect(screen.getByRole("button", { name: "Write email" })).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Draft the reply" }),
    ).toBeNull();
  });

  it("carries the caller's own wording when overrides are supplied", () => {
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <RecordEmailAside
            entityType="deal"
            entityId={PERSON}
            strings={{
              title: "dealmail.title",
              subReply: "dealmail.sub.reply",
              subFresh: "dealmail.sub.fresh",
              reply: "dealmail.reply",
              send: "dealmail.send",
            }}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(screen.getByRole("button", { name: "Send an email" })).toBeTruthy();
  });
});

// `detectWaitingReply` is the box's OWN read of the same question DealEmailAside
// already answers from the deal status card — the path a person or lead page
// takes because it has none. These mock the server at the fetch boundary
// rather than the prop, so they cover the read itself and not just its wiring.
describe("the record's own waiting-reply read", () => {
  function renderDetecting() {
    return render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <RecordEmailAside
            entityType="person"
            entityId={PERSON}
            detectWaitingReply
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
  }

  it("shows fresh wording while the read is still unsettled", () => {
    // A promise that never resolves stands in for "in flight": the panel must
    // draw the safe half from the first render, not wait for an answer that
    // has not come back yet.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    renderDetecting();
    expect(screen.getByRole("button", { name: "Write email" })).toBeTruthy();
  });

  it("offers fresh mail when nothing on the record is waiting", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    renderDetecting();
    expect(
      await screen.findByRole("button", { name: "Write email" }),
    ).toBeTruthy();
  });

  it("anchors the composer to the newest activity the read finds", async () => {
    const requestedUrls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
          "https://test.local",
        );
        requestedUrls.push(url.pathname);
        if (url.pathname.endsWith("/activities")) {
          return jsonResponse({
            data: [{ id: MAIL }],
            page: { next_cursor: null, has_more: false },
          });
        }
        if (url.pathname.endsWith(`/activities/${MAIL}/reply-recipient`)) {
          return jsonResponse({ full_name: "", first_name: "", address: "" });
        }
        if (url.pathname.endsWith("/consent-purposes")) {
          return jsonResponse({
            data: [],
            page: { next_cursor: null, has_more: false },
          });
        }
        if (url.pathname.endsWith("/voice-profiles")) {
          return jsonResponse({
            data: [],
            page: { next_cursor: null, has_more: false },
          });
        }
        return jsonResponse({});
      }),
    );
    const user = userEvent.setup();
    renderDetecting();
    const replyButton = await screen.findByRole("button", {
      name: "Draft the reply",
    });
    await user.click(replyButton);
    // The composer only asks who a reply goes to for the activity it was
    // opened against, so this request naming MAIL is the proof that the
    // hook's id, not just its presence, reached ComposeModal's `activityId`.
    await screen.findByRole("dialog");
    expect(
      requestedUrls.some((p) =>
        p.endsWith(`/activities/${MAIL}/reply-recipient`),
      ),
    ).toBe(true);
  });
});
