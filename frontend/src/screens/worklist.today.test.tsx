// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// One morning, drawn ONCE.
//
// The queue was the ranking's answer to what matters most, and the page said it
// three times: a focus card repeated the top row whole, a Next-up list repeated
// the three after it as titles, and the queue below drew all four again. A
// reader counting their morning off this screen counted some of it twice.
//
// So the panel IS the answer, and the first row is the focus because it is
// first. These tests hold the two things that had to survive the fold: the
// first row is in hand on arrival, and a task can still be finished in one
// press from where the reader is standing.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aTask(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-000000000099",
    source: "task",
    category: "tasks",
    title: "Call Alice back",
    band: "now",
    actions: ["complete", "snooze"],
    primary_action: "complete",
    subject: {
      type: "person",
      id: "01a05500-0000-7000-8000-000000000011",
      label: "Alice Müller",
    },
    ...over,
  });
}

describe("the day is one panel", () => {
  it("draws no second copy of the top row", async () => {
    stub(
      day({
        queue: [aTask({ title: "Call Alice back" })],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    // The title appears ONCE. While the focus card stood above the queue this
    // row was drawn twice on one screen, and the assertion that catches a
    // regression is the count rather than the presence.
    await waitFor(() => {
      expect(screen.getAllByText(/Call Alice back/)).toHaveLength(1);
    });
  });

  it("names the panel for the day rather than for what comes next", async () => {
    stub(
      day({
        queue: [aTask()],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    // Three panels each claimed to say what to do next — "Do this next", "And
    // then", "What to do next". ONE list now, and this counts the LISTS rather
    // than naming the strings those panels used: their words are gone from
    // every catalog, so asserting their absence tests nothing a regression
    // could fail. A focus card returning would draw its row outside this list.
    await waitFor(() => {
      expect(screen.getByText("Today")).toBeTruthy();
    });
    const lists = document.querySelectorAll("ol.worklist-list");
    expect(lists.length).toBe(1);
  });
});

describe("a task is finished where the reader is standing", () => {
  it("submits once however fast the reader presses", async () => {
    let patches = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : undefined;
        const url = String(request?.url ?? input);
        if (request?.method === "PATCH") {
          patches += 1;
          // HELD, the way a real network holds it. A PATCH that resolves in
          // the same tick never lets the button paint its busy state, so the
          // test would be measuring the mock rather than the guard.
          await new Promise((settle) => setTimeout(settle, 20));
          return new Response(null, { status: 204 });
        }
        if (url.includes("/worklist")) {
          return new Response(
            JSON.stringify(
              day({
                queue: [aTask()],
                summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
              }),
            ),
            { status: 200, headers: { "content-type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [] }), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }),
    );
    renderWorklist();

    const done = await screen.findByRole("button", { name: "Done" });
    // Two presses with no wait between them. The mutation holds the button
    // pending until the refetch it triggered has settled, so the second press
    // lands on a disabled control — without that the row sits there finished
    // and pressable, and the second PATCH answers for a task already done.
    await userEvent.click(done);
    await userEvent.click(done);

    // ONE write, however many presses. `Button` drops its `onClick` while
    // `pending`, and the mutation stays pending until the refetch it triggered
    // has settled — so the finished row is never both on screen and pressable.
    await waitFor(() => {
      expect(patches).toBe(1);
    });
    // And the write did land: an assertion that only counts PATCHes passes just
    // as well against a button that never fired at all.
    expect(patches).toBe(1);
  });

  it("completes it rather than navigating to it", async () => {
    const fetched = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/worklist")) {
        return new Response(
          JSON.stringify(
            day({
              queue: [aTask()],
              summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
            }),
          ),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      if (url.includes("/activities/")) {
        return new Response(null, { status: 204 });
      }
      return new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetched);
    renderWorklist();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
    });
    await userEvent.click(screen.getByRole("button", { name: "Done" }));

    // The ADDRESS, the METHOD and the BODY — all three, because each one alone
    // passes over a different broken button. `VERB_DESTINATION` routes
    // `complete` to the task's own record, so a control that merely navigated
    // would render identically and leave the task open; and a PATCH carrying
    // `is_done: false` reaches the same address by the same method and REOPENS
    // the task the reader just finished. Asserting the first two alone let that
    // second one through, which is why the body is read here.
    await waitFor(() => {
      const completions = fetched.mock.calls
        .map(([input]) => (input instanceof Request ? input : undefined))
        .filter((request) => request?.method === "PATCH")
        .filter((request) =>
          request?.url.includes(
            "/activities/01a05500-0000-7000-8000-000000000099",
          ),
        );
      expect(completions.length).toBeGreaterThan(0);
      return completions;
    });
    const patched = fetched.mock.calls
      .map(([input]) => (input instanceof Request ? input : undefined))
      .find(
        (request) =>
          request?.method === "PATCH" &&
          request.url.includes(
            "/activities/01a05500-0000-7000-8000-000000000099",
          ),
      );
    expect(await patched?.clone().json()).toEqual({ is_done: true });
  });
});
