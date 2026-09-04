// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  stub,
} from "./worklist.testkit";

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

  // Two presses while the FIRST IS STILL IN FLIGHT.
  //
  // The write is held open here rather than answered at once, and that is the
  // whole point: against an instant stub the first POST has already succeeded
  // by the time the second press lands, so `isSuccess` alone stops it and the
  // `isPending` half of the guard can be deleted with every test still green.
  // A real POST takes long enough for a rep to press twice.
  it("submits once however fast the reader presses", async () => {
    const held = stubHeldWrite();
    renderWorklist();

    const done = await screen.findByRole("button", { name: "Done" });
    await userEvent.click(done);
    await userEvent.click(done);
    held.release();

    await waitFor(() => {
      expect(writes(held.fetched)).toHaveLength(1);
    });
  });

  // A press AFTER the write settles, before the refetch replaces the row.
  //
  // This is the window `isSuccess` exists for, and it is the reason a pending
  // check alone is not enough: a completed task LEAVES the queue and takes its
  // button with it, while an answered brief item is patched in place and the
  // row is still on screen. Hold the refetch open and the row keeps its live
  // buttons over an item that has already been answered.
  it("submits once when the reader presses again before the row refreshes", async () => {
    const held = stubHeldRefetch();
    renderWorklist();

    const done = await screen.findByRole("button", { name: "Done" });
    await userEvent.click(done);
    // The write has settled: one POST is on the record.
    await waitFor(() => {
      expect(writes(held.fetched)).toHaveLength(1);
    });
    await userEvent.click(done);
    held.release();

    await waitFor(() => {
      expect(writes(held.fetched)).toHaveLength(1);
    });
  });
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

  // The SERVER decides which of the three a row gets.
  //
  // Every other case here sends all three, so none of them can tell a client
  // reading `actions` from one that assumes the lane always sends the full
  // set. The day one is withheld, the assuming client keeps drawing it and
  // posts an answer the server did not offer.
  it("draws only the verbs the row was given", async () => {
    stub(
      day({
        queue: [aBriefItem({ actions: ["act"], primary_action: "act" })],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(await screen.findByRole("button", { name: "Done" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Snooze" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });

  // A refusal names its CAUSE.
  //
  // The failure handler used to read `mark.error`, which is the state React
  // last rendered rather than the error the callback was handed — on a first
  // failure that field is still null. So a rep whose colleague had already
  // answered the item in another tab was told "no cause reported", and the
  // retry that invites hits the same conflict every time.
  it("names why an answer was refused", async () => {
    stubRefusedAnswer();
    renderUnderAToastRegion();

    await userEvent.click(await screen.findByRole("button", { name: "Done" }));

    expect(await screen.findByText(A_CONFLICT)).toBeTruthy();
  });
});

const A_CONFLICT = "Somebody already answered this item.";

// stubRefusedAnswer serves the row, then refuses the answer with a cause.
//
// The cause is what makes this test able to fail: a handler reading the stale
// `mark.error` falls back to the catalog's "no cause reported", which is a
// different string from this one.
function stubRefusedAnswer() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/brief/items/")) {
        return new Response(
          JSON.stringify({
            title: "Conflict",
            status: 409,
            detail: A_CONFLICT,
          }),
          { status: 409, headers: { "content-type": "application/json" } },
        );
      }
      if (url.split("?")[0].endsWith("/worklist")) {
        return jsonResponse(
          day({
            queue: [aBriefItem()],
            summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
          }),
        );
      }
      return jsonResponse({ data: [] });
    }),
  );
}

// renderUnderAToastRegion draws the screen the way the shell draws it: the
// refusal above appears in a toast, and `renderWorklist` mounts no region —
// deliberately, since the conformance gate allows exactly one and names
// main.tsx as its home. A test file the gate does not scan is where it goes.
function renderUnderAToastRegion() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// stubHeldRefetch answers the POST at once and holds the worklist read that
// FOLLOWS it, so the row stays on screen with its answer already recorded.
function stubHeldRefetch(): {
  fetched: ReturnType<typeof vi.fn>;
  release: () => void;
} {
  let release = () => {};
  const refetched = new Promise<void>((resolve) => {
    release = resolve;
  });
  let written = false;
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : undefined;
    const url = String(request?.url ?? input);
    if (request?.method === "POST") {
      written = true;
      return jsonResponse({ id: "01a05500-0000-7000-8000-0000000000b1" });
    }
    if (url.includes("/worklist")) {
      if (written) {
        await refetched;
      }
      return jsonResponse(aDayOfOneBriefItem());
    }
    return jsonResponse({ data: [] });
  });
  vi.stubGlobal("fetch", fetched);
  return { fetched, release };
}

// stubHeldWrite answers reads at once and holds the POST until released, so a
// second press lands while the first write is still in flight.
function stubHeldWrite(): {
  fetched: ReturnType<typeof vi.fn>;
  release: () => void;
} {
  let release = () => {};
  const inFlight = new Promise<void>((resolve) => {
    release = resolve;
  });
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : undefined;
    const url = String(request?.url ?? input);
    if (request?.method === "POST") {
      await inFlight;
      return jsonResponse({ id: "01a05500-0000-7000-8000-0000000000b1" });
    }
    if (url.includes("/worklist")) {
      return jsonResponse(aDayOfOneBriefItem());
    }
    return jsonResponse({ data: [] });
  });
  vi.stubGlobal("fetch", fetched);
  return { fetched, release };
}

// A folded group stands for a pile and names no single brief item, so it
