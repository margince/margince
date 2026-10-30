// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// Scrolling the Worklist to its end, on the real screen. The fetch and the
// browser's IntersectionObserver are the boundaries; each test says what they
// answer and when.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  type Worklist,
} from "../../testing/worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

/** The query of one `/worklist` read. */
function worklistParams(input: RequestInfo | URL): URLSearchParams | null {
  const url = new URL(
    input instanceof Request ? input.url : String(input),
    "http://localhost",
  );
  return url.pathname.endsWith("/worklist") ? url.searchParams : null;
}

/** Serves one page per cursor and records every cursor asked for. */
function serveByCursor(pages: Readonly<Record<string, Worklist>>): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const params = worklistParams(input);
      if (!params) return jsonResponse({ data: [] });
      const cursor = params.get("cursor") ?? "";
      asked.push(cursor);
      const page = pages[cursor];
      if (!page) return new Response(null, { status: 503 });
      return jsonResponse(page);
    }),
  );
  return asked;
}

// A viewport so short that the end of the list is always in view. The browser
// reports that on every observe, and here it reports it twice.
class ShortViewport {
  constructor(private readonly callback: IntersectionObserverCallback) {}
  private connected = true;
  observe() {
    queueMicrotask(() => {
      for (const _ of [1, 2]) {
        if (!this.connected) return;
        const entry = { isIntersecting: true } as IntersectionObserverEntry;
        this.callback([entry], this as unknown as IntersectionObserver);
      }
    });
  }
  unobserve() {}
  takeRecords() {
    return [];
  }
  disconnect() {
    this.connected = false;
  }
}

describe("a list shorter than the screen", () => {
  it("loads each batch once, walks past an empty batch, and stops at the end", async () => {
    vi.stubGlobal("IntersectionObserver", ShortViewport);
    const asked = serveByCursor({
      "": day({
        queue: [row({ id: "a", title: "First task" })],
        next_cursor: "c1",
      }),
      c1: day({ queue: [], next_cursor: "c2" }),
      c2: day({ queue: [row({ id: "b", title: "Last task" })] }),
    });
    renderWorklist();

    await screen.findByText(en["worklist.end.today"]);
    expect(screen.getByText("First task")).toBeTruthy();
    expect(screen.getByText("Last task")).toBeTruthy();
    expect(asked).toEqual(["", "c1", "c2"]);
  });
});

describe("a continuation that fails", () => {
  it("keeps the loaded rows in place, and Retry adds the next batch once", async () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    let failures = 1;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const params = worklistParams(input);
        if (!params) return jsonResponse({ data: [] });
        if (!params.get("cursor")) {
          return jsonResponse(
            day({
              queue: [
                row({ id: "a", title: "Call the buyer" }),
                row({ id: "b", title: "Send the quote" }),
              ],
              next_cursor: "c1",
            }),
          );
        }
        if (failures-- > 0) return new Response(null, { status: 503 });
        return jsonResponse(
          day({
            queue: [
              row({ id: "b", title: "Send the quote" }),
              row({ id: "c", title: "Book the demo" }),
            ],
          }),
        );
      }),
    );
    renderWorklist();
    const first = await screen.findByText("Call the buyer");

    await userEvent.click(
      screen.getByRole("button", { name: en["list.loadMore"] }),
    );
    await screen.findByText(en["scrollSentinel.failed"]);
    expect(screen.getByText("Call the buyer")).toBe(first);
    // Without an IntersectionObserver the sentinel's retry is Load more again.
    await userEvent.click(
      screen.getByRole("button", { name: en["list.loadMore"] }),
    );

    await screen.findByText("Book the demo");
    expect(screen.getByText("Call the buyer")).toBe(first);
    expect(screen.getAllByText("Send the quote")).toHaveLength(1);
    const titles = ["Call the buyer", "Send the quote", "Book the demo"].map(
      (title) => screen.getByText(title),
    );
    for (let i = 1; i < titles.length; i++) {
      expect(
        titles[i - 1].compareDocumentPosition(titles[i]) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
  });
});

type Held = Readonly<{
  params: URLSearchParams;
  signal: AbortSignal;
  answer: (page: Worklist) => void;
}>;

// The team board a team scope reads beside the list, with the reader alone on it.
const QUIET_TEAM = {
  as_of: "2026-08-31T09:00:00Z",
  members: [
    {
      user_id: "00000000-0000-7000-8000-000000000001",
      display_name: "The reader",
      counts: { waiting: 0, at_risk: 0, overdue: 0 },
    },
  ],
  unassigned: { waiting: 0, at_risk: 0, overdue: 0 },
  truncated: false,
};

/** Holds every `/worklist` read until the test answers it. */
function holdReads(): Held[] {
  const held: Held[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const params = worklistParams(request.url);
      if (request.url.split("?")[0]?.endsWith("/worklist/team")) {
        return Promise.resolve(jsonResponse(QUIET_TEAM));
      }
      if (!params) return Promise.resolve(jsonResponse({ data: [] }));
      return new Promise<Response>((resolve) => {
        held.push({
          params,
          signal: request.signal,
          answer: (page) => resolve(jsonResponse(page)),
        });
      });
    }),
  );
  return held;
}

// The reader asks for Today's next batch, then moves to another list before
// it answers. Each case is one way to move.
const moves = [
  {
    name: "another view",
    press: () => screen.getByRole("button", { name: /^Decisions/ }),
    asked: (params: URLSearchParams) => params.get("view") === "decisions",
    category: "decisions",
  },
  {
    name: "another filter",
    press: () =>
      screen.getByRole("button", { name: en["worklist.filter.tasks"] }),
    asked: (params: URLSearchParams) => params.get("filter") === "tasks",
    category: "tasks",
  },
  {
    name: "another owner scope",
    press: () =>
      screen.getByRole("button", { name: en["worklist.scope.team"] }),
    asked: (params: URLSearchParams) => params.get("scope") === "team",
    category: "tasks",
  },
] as const;

describe("a late answer for the list the reader left", () => {
  it.each(moves)(
    "never reaches the list now on screen after moving to $name",
    async ({ press, asked, category }) => {
      vi.stubGlobal("IntersectionObserver", undefined);
      const held = holdReads();
      renderWorklist();
      await waitFor(() => expect(held).toHaveLength(1));
      held[0].answer(
        day({
          queue: [row({ id: "a", title: "Today's first task" })],
          scope_options: ["mine", "team"],
          next_cursor: "c1",
        }),
      );
      await userEvent.click(
        await screen.findByRole("button", { name: en["list.loadMore"] }),
      );
      await waitFor(() => expect(held).toHaveLength(2));
      const continuation = held[1];
      expect(continuation.params.get("cursor")).toBe("c1");

      await userEvent.click(press());
      await waitFor(() =>
        expect(held.some((read) => asked(read.params))).toBe(true),
      );
      const moved = held.find((read) => asked(read.params));
      moved?.answer(
        day({
          view:
            moved.params.get("view") === "decisions" ? "decisions" : "today",
          queue: [row({ id: "n", category, title: "The new list's row" })],
        }),
      );
      await screen.findByText("The new list's row");
      continuation.answer(
        day({ queue: [row({ id: "late", title: "A late row" })] }),
      );

      await waitFor(() => expect(continuation.signal.aborted).toBe(true));
      expect(screen.queryByText("A late row")).toBeNull();
      expect(screen.queryByText("Today's first task")).toBeNull();
      expect(held.filter((read) => asked(read.params))).toHaveLength(1);
    },
  );
});
