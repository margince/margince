// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Answering a waiting buyer from the row that named the wait.
//
// The row a rep meets most often is somebody who wrote and nobody answered, and
// it used to send them to the record to press Reply there.

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { draftButton } from "../../testing/composedraft.testkit";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  stub,
} from "../../testing/worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aWaitingBuyer(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000c1",
    source: "customer_waiting",
    category: "customer_waiting",
    title: "can you resend the quote?",
    band: "now",
    destination: "today",
    actions: ["open", "reply"],
    subject: { type: "contact", id: "01a05500-0000-7000-8000-0000000000c2" },
    ...over,
  });
}

function aDayWith(item: ReturnType<typeof aWaitingBuyer>) {
  return day({
    queue: [item],
    summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
  });
}

describe("a buyer waiting on a reply", () => {
  it("can be answered from the row", async () => {
    stub(aDayWith(aWaitingBuyer()));
    renderWorklist();

    await screen.findByText(/can you resend the quote/);
    expect(
      await screen.findByRole("button", { name: /^reply$/i }),
    ).not.toBeNull();
  });

  it("offers no reply when the server sent none", async () => {
    // The server withholds the verb for a channel message and for a wait that
    // names no record. Either way the row must draw nothing rather than a
    // composer that would answer in the wrong place or file nowhere.
    stub(aDayWith(aWaitingBuyer({ actions: ["open"] })));
    renderWorklist();

    await screen.findByText(/can you resend the quote/);
    expect(screen.queryByRole("button", { name: /^reply$/i })).toBeNull();
  });

  it("offers no reply when the row names no record to file against", async () => {
    // A message from a stranger. The verb alone is not enough: the composer
    // links the sent message to a record, and the row's subject is what says
    // which. Without it there is nothing to open the composer on.
    stub(aDayWith(aWaitingBuyer({ subject: undefined })));
    renderWorklist();

    await screen.findByText(/can you resend the quote/);
    expect(screen.queryByRole("button", { name: /^reply$/i })).toBeNull();
  });
});

describe("a message the reader sent and nobody answered", () => {
  it("offers to draft the follow-up from the row", async () => {
    stub(
      aDayWith(
        aWaitingBuyer({
          source: "awaiting_reply",
          category: "tasks",
          title: "the proposal we discussed",
          move: {
            action: "draft_reply",
            activity_id: "01a05500-0000-7000-8000-0000000000c1",
          },
        }),
      ),
    );
    renderWorklist();

    await screen.findByText(/the proposal we discussed/);
    expect(
      await screen.findByRole("button", { name: /draft follow-up/i }),
    ).not.toBeNull();
  });
});

describe("a meeting the reader has sent nothing after", () => {
  it("offers to draft a follow-up to the contact they met", async () => {
    stub(
      aDayWith(
        aWaitingBuyer({
          source: "meeting_follow_up",
          category: "meetings",
          title: "Discovery workshop",
          contact: { id: "01a05500-0000-7000-8000-0000000000c2" },
        }),
      ),
    );
    renderWorklist();

    await screen.findByText(/Discovery workshop/);
    const draft = await screen.findByRole("button", {
      name: /draft follow-up/i,
    });
    expect(screen.queryByRole("button", { name: /^reply$/i })).toBeNull();
    await userEvent.click(draft);
    expect(await screen.findByRole("dialog")).not.toBeNull();
  });

  it("drafts one message to everyone the meeting was with", async () => {
    const [first, second, third] = [
      "01a05500-0000-7000-8000-0000000000d1",
      "01a05500-0000-7000-8000-0000000000d2",
      "01a05500-0000-7000-8000-0000000000d3",
    ];
    const everyone = [
      "d1@customer.example",
      "d2@customer.example",
      "d3@customer.example",
    ];
    stub(
      aDayWith(
        aWaitingBuyer({
          source: "meeting_follow_up",
          category: "meetings",
          title: "Pricing review",
          subject: { type: "deal", id: "01a05500-0000-7000-8000-0000000000e1" },
          contact: { id: first },
          recipients: [{ id: first }, { id: second }, { id: third }],
        }),
      ),
    );
    const served = globalThis.fetch;
    const drafts: { url: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const url = String(request ? request.url : input);
        if (!url.includes("/draft-email")) return served(input, init);
        const body: unknown = request
          ? await request.clone().json()
          : JSON.parse(String(init?.body));
        drafts.push({ url, body });
        return jsonResponse({
          subject: "Thanks for today",
          body: "Hello all,",
          generated_by: "deterministic",
          to: everyone,
        });
      }),
    );
    renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: /draft follow-up/i }),
    );
    await userEvent.click(await draftButton());
    await screen.findByDisplayValue("Thanks for today");
    expect(drafts[0]?.url).toContain(`/contacts/${first}/draft-email`);
    expect(drafts[0]?.body).toMatchObject({
      also_to: [second, third],
      about_activity_id: "01a05500-0000-7000-8000-0000000000c1",
    });
    for (const address of everyone) {
      expect(await screen.findByText(address)).toBeTruthy();
    }
  });

  it("opens its record when no attendee is on file", async () => {
    stub(
      aDayWith(
        aWaitingBuyer({
          source: "meeting_follow_up",
          category: "meetings",
          title: "Pricing review",
          actions: ["open"],
          subject: { type: "deal", id: "01a05500-0000-7000-8000-0000000000e1" },
          because: [{ kind: "met_days_ago" }, { kind: "no_attendee_on_file" }],
        }),
      ),
    );
    renderWorklist();

    // The title links the deal, so the row needs no Open of its own.
    const title = await screen.findByRole("link", { name: /Pricing review/ });
    expect(title.getAttribute("href")).toContain(
      "01a05500-0000-7000-8000-0000000000e1",
    );
    expect(
      screen.queryByRole("button", { name: /draft follow-up/i }),
    ).toBeNull();
  });
});
