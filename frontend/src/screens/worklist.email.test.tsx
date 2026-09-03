// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// What a waiting EMAIL looks like on the queue, and what every other row keeps.
//
// The lane spans email and channel messages, so these are two claims about one
// page: an email names itself with the canonical row, and everything else — a
// chat, a task, a drifting deal — reads exactly as it did. Either alone passes
// for the wrong reason.
//
// Through the SCREEN rather than the row component, because the wiring is the
// half that breaks: the opener is threaded from the page that owns the drawer,
// down through the body, to the row. A row rendered on its own would prove the
// component and nothing about whether the page reaches it.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  stub,
  type Worklist,
  type WorklistItem,
} from "./worklist.testkit";

afterEach(() => {
  vi.unstubAllGlobals();
});

const MESSAGE_ID = "01a05500-0000-7000-8000-0000000000e1";

const waitingEmail: WorklistItem = row({
  id: MESSAGE_ID,
  source: "customer_waiting",
  category: "customer_waiting",
  level: 1,
  consequence: "buyer_waits",
  title: "Re: the renewal quote",
  email_summary: {
    activity_id: MESSAGE_ID,
    occurred_at: "2026-08-29T09:15:00Z",
    version: 4,
    subject: "Re: the renewal quote",
    preview: "Can you hold the price until Friday?",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
  },
});

// A chat waiting in the SAME lane. The server sends it no email row, so the
// queue keeps the plain title it had — a chat drawn as an email would carry a
// mail icon and an access badge over a message that never travelled on one.
const waitingChat: WorklistItem = row({
  id: "01a05500-0000-7000-8000-0000000000e2",
  source: "customer_waiting",
  category: "customer_waiting",
  level: 1,
  consequence: "buyer_waits",
  title: "ping about the quote",
});

// The queue's own stub answers `{data: []}` to everything but the worklist,
// which the email drawer reads as a presentation with no access block and
// throws on. This serves the detail read too, so opening a message from the
// queue is testable without the drawer's own contract moving into this file.
const presentation = {
  id: MESSAGE_ID,
  lifecycle: "delivered",
  occurred_at: "2026-08-29T09:15:00Z",
  summary: waitingEmail.email_summary,
  body: "Can you hold the price until Friday?",
  thread_key: "t1",
  from: [{ address: "dana@acme.example", display_name: "Dana Buyer" }],
  to: [],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
  links: [],
  thread: { members: [], next_cursor: null },
  access: {
    content_state: "available",
    audience: "workspace",
    selected_members: [],
    display_status: "team",
    can_change: false,
    change_mode: "message",
    held_by_others: false,
  },
  can_reply: true,
  can_relink: false,
  version: 4,
};

function renderWithQueue(queue: Worklist) {
  stub(queue);
  return renderWorklist();
}

function stubWithDrawer(queue: Worklist) {
  stub(queue);
  // Typed as fetch itself, not as a Mock: the queue's stub installs one, and a
  // Mock's own type is not callable without a cast the linter refuses.
  const worklistFetch = globalThis.fetch;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("email-presentation")) {
        return jsonResponse(presentation);
      }
      return worklistFetch(input, init);
    }),
  );
}

describe("a waiting row on the queue", () => {
  it("names a waiting email with the canonical row and opens the message", async () => {
    stubWithDrawer(day({ queue: [waitingEmail] }));
    renderWorklist();

    await waitFor(() =>
      expect(
        screen.getAllByText("Re: the renewal quote").length,
      ).toBeGreaterThan(0),
    );
    // The server's own preview — the same line the timeline draws, not a slice
    // this screen cut for itself.
    expect(
      screen.getAllByText("Can you hold the price until Friday?").length,
    ).toBeGreaterThan(0);
    // The access badge rides the row, so a rep tells a limited conversation
    // from an open one without opening it.
    expect(screen.getAllByText("Team").length).toBeGreaterThan(0);

    const [opener] = screen.getAllByRole("button", {
      name: /Re: the renewal quote/,
    });
    expect(opener.getAttribute("aria-haspopup")).toBe("dialog");
    await userEvent.click(opener);
    // The page's own drawer, over the queue.
    await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
  });

  it("keeps the queue's own furniture beside the email row", async () => {
    // TWO rows: the first becomes the focus card above the queue, so a
    // single-item queue draws no row at all and the claim below would be about
    // a list that is not there.
    const { container } = renderWithQueue(
      day({ queue: [waitingChat, waitingEmail] }),
    );

    // Waits for the ROW, not for the subject: the focus card above the queue
    // draws the same message and would satisfy a subject wait while the list
    // below was still rendering.
    await waitFor(() => {
      if (!container.querySelector(".worklist-row")) {
        throw new Error("the queue has not drawn its rows yet");
      }
    });
    // Asked of the ROW, not the page: the same words label the filter control
    // above, and a page-wide match would pass on that one while the row lost
    // its badge. The category says where the row sits in the DAY, which the
    // email row does not answer — a row that traded it for a subject would
    // lose the reason it is on this page at all.
    // Queried AFTER the wait above: the container is captured at render, and
    // reading it before the page has answered finds a loading skeleton.
    const rowEl = container.querySelector(".worklist-row");
    if (!rowEl) {
      throw new Error("the queue drew no row at all");
    }
    expect(rowEl.textContent).toContain("Customer waiting");
  });

  it("leaves a waiting chat on the plain title it had", async () => {
    stub(day({ queue: [waitingChat] }));
    renderWorklist();

    await waitFor(() =>
      expect(screen.getByText("ping about the quote")).toBeTruthy(),
    );
    // Nothing openable: there is no email drawer for a chat, and a control
    // that answers nothing teaches a rep the product is broken.
    expect(
      screen.queryByRole("button", { name: /ping about the quote/ }),
    ).toBeNull();
  });
});
