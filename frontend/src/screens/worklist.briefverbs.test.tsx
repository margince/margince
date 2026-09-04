// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// A brief item is seller work, and it is answerable where it is ranked.
//
// `brief_item` is classified `today`, so it lands on a rep's own screen among
// the replies and the tasks. The server sends `act`, `set_aside` and `dismiss`
// with it and none of the three is in VERB_DESTINATION, so the queue drew a
// title, a deal and a Pin — a row naming the rep's most important next move
// with no way to make it.
//
// These tests assert the ADDRESS each verb posts to, not its label. The three
// go to different endpoints and a control wired to the wrong one renders
// identically to a control wired to the right one.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aBriefItem(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000b1",
    source: "brief_item",
    category: "deals_at_risk",
    title: "Advance the Northstar deal",
    band: "now",
    destination: "today",
    actions: ["act", "set_aside", "dismiss"],
    primary_action: "act",
    subject: {
      type: "deal",
      id: "01a05500-0000-7000-8000-0000000000bb",
      label: "Northstar",
    },
    ...over,
  });
}

function aDayOfOneBriefItem() {
  return day({
    queue: [aBriefItem()],
    summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
  });
}

// Every POST the page made, as "METHOD /path" — the shape a wrong endpoint
// shows up in.
function writes(fetched: ReturnType<typeof vi.fn>): string[] {
  return fetched.mock.calls
    .map(([input]) => {
      const request = input instanceof Request ? input : undefined;
      if (request?.method !== "POST") {
        return null;
      }
      return `POST ${new URL(request.url).pathname}`;
    })
    .filter((route): route is string => route !== null);
}

function stubWrites(): ReturnType<typeof vi.fn> {
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : undefined;
    const url = String(request?.url ?? input);
    if (request?.method === "POST") {
      return new Response(
        JSON.stringify({ id: "01a05500-0000-7000-8000-0000000000b1" }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      );
    }
    if (url.includes("/worklist")) {
      return new Response(JSON.stringify(aDayOfOneBriefItem()), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    return new Response(JSON.stringify({ data: [] }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

describe("a brief item is answerable from the queue", () => {
  it("offers the three verbs the server sent", async () => {
    stub(aDayOfOneBriefItem());
    renderWorklist();

    await screen.findByText(/Advance the Northstar deal/);
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Snooze" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeTruthy();
  });

  it("acts through the brief's own act endpoint", async () => {
    const fetched = stubWrites();
    renderWorklist();

    await userEvent.click(await screen.findByRole("button", { name: "Done" }));

    await waitFor(() => {
      expect(writes(fetched)).toEqual([
        "POST /v1/brief/items/01a05500-0000-7000-8000-0000000000b1/act",
      ]);
    });
  });

  // `set_aside` is the BRIEF's snooze, not a task's.
  //
  // A task's snooze moves a due date the rep agreed to; a brief item's hides a
  // suggestion until later in the day. The contract names the difference, and
  // one word for both is exactly how a client writes the wrong endpoint — so
  // the address is what this asserts.
  it("sets aside through the brief's snooze, not a task's", async () => {
    const fetched = stubWrites();
    renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: "Snooze" }),
    );

    await waitFor(() => {
      expect(writes(fetched)).toEqual([
        "POST /v1/brief/items/01a05500-0000-7000-8000-0000000000b1/snooze",
      ]);
    });
  });

  it("dismisses through the brief's own dismiss endpoint", async () => {
    const fetched = stubWrites();
    renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: "Dismiss" }),
    );

    await waitFor(() => {
      expect(writes(fetched)).toEqual([
        "POST /v1/brief/items/01a05500-0000-7000-8000-0000000000b1/dismiss",
      ]);
    });
  });

  it("submits once however fast the reader presses", async () => {
    const fetched = stubWrites();
    renderWorklist();

    const done = await screen.findByRole("button", { name: "Done" });
    await userEvent.click(done);
    await userEvent.click(done);

    await waitFor(() => {
      expect(writes(fetched)).toHaveLength(1);
    });
  });

  // A folded group stands for a pile and names no single brief item, so it
  // gets no verb that would answer one of them.
  it("draws no verb on a folded group", async () => {
    stub(
      day({
        queue: [
          aBriefItem({
            batch: {
              key: "brief_pile",
              count: 4,
              label: "Four deals to advance",
            },
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The row is on the page — waited for through the queue itself, since a
    // folded group's title is composed from the batch rather than sent whole.
    await waitFor(() => {
      expect(document.querySelectorAll(".worklist-list li")).toHaveLength(1);
    });
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });
});
