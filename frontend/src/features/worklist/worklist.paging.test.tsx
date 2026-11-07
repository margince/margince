// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { de } from "../../i18n/de";
import { en } from "../../i18n/en";
import { vi as viCatalog } from "../../i18n/vi";
import {
  day,
  renderWorklist,
  row,
  stub,
  stubWalk,
} from "../../testing/worklist.testkit";

// Without an IntersectionObserver the scroll sentinel offers its button; a
// browser loads the same batch when the end of the list shows.
const MORE = en["list.loadMore"];
beforeEach(() => vi.stubGlobal("IntersectionObserver", undefined));

// Reaching the whole backlog.
//
// The page used to stop at its first read with no route to what was behind it:
// the header said work existed and the queue offered no way to it. These cases
// are about the two halves of fixing that — the rows accumulate, and the day's
// own figures do not move while they do.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("walking to the rest of the queue", () => {
  it("loads the next page and keeps what was already read", async () => {
    stubWalk([
      day({
        queue: [row({ id: "a", title: "First thing" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
        next_cursor: "page-2",
      }),
      day({
        queue: [row({ id: "b", title: "Second thing" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    ]);
    renderWorklist();

    await screen.findByText("First thing");
    await userEvent.click(screen.getByRole("button", { name: MORE }));

    // BOTH, not just the newest page: a walk that replaced the list would lose
    // the rows the reader already worked through.
    await screen.findByText("Second thing");
    expect(screen.getByText("First thing")).toBeTruthy();
  });

  it("offers no way on when the walk is finished", async () => {
    stub(
      day({
        queue: [row({ id: "a", title: "Only thing" })],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    await screen.findByText("Only thing");
    // A final page carries no cursor. A control offered here would ask the
    // server for a page it already said does not exist.
    expect(screen.queryByRole("button", { name: MORE })).toBeNull();
  });

  // THE case the contract warns about. A walk is not a snapshot: the day is
  // re-assembled and re-ranked on every read, so a row crossing the page
  // boundary between two reads is served twice. React would render duplicate
  // keys for it, and the reader would see the same work listed twice.
  it("draws a row served on two pages once", async () => {
    const straddler = row({ id: "a", title: "Served twice" });
    stubWalk([
      day({
        queue: [straddler],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
        next_cursor: "page-2",
      }),
      day({
        queue: [straddler, row({ id: "b", title: "Genuinely new" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    ]);
    renderWorklist();

    await screen.findByText("Served twice");
    await userEvent.click(screen.getByRole("button", { name: MORE }));

    await screen.findByText("Genuinely new");
    expect(screen.getAllByText("Served twice")).toHaveLength(1);
  });

  // Two lanes mint ids independently, so the same id in two sources is two
  // different pieces of work. Deduping on the id alone would silently drop the
  // second one.
  it("keeps two rows that share an id across different sources", async () => {
    stubWalk([
      day({
        queue: [row({ id: "same", source: "task", title: "A task" })],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
        next_cursor: "page-2",
      }),
      day({
        queue: [
          row({
            id: "same",
            source: "notice",
            category: "system",
            title: "A notice",
          }),
        ],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    ]);
    renderWorklist();

    await screen.findByText("A task");
    await userEvent.click(screen.getByRole("button", { name: MORE }));

    await screen.findByText("A notice");
    expect(screen.getByText("A task")).toBeTruthy();
  });

  // The headline is the view's count from the first page, counted by the
  // server over the whole view. A later page's own figures never reach it.
  it("AC-WORKLIST-SDR-04: keeps the view's figure still while the rows grow", async () => {
    const counts = [
      {
        category: "tasks" as const,
        considered: 48,
        shown: 1,
        more_available: false,
      },
    ];
    stubWalk([
      day({
        queue: [row({ id: "a", title: "First thing" })],
        summary: { urgent: 3, due: 9, lower_priority: 6, total: 48 },
        counts,
        next_cursor: "page-2",
      }),
      day({
        queue: [row({ id: "b", title: "Second thing" })],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
        counts: [{ ...counts[0], considered: 1 }],
      }),
    ]);
    renderWorklist();

    await screen.findByText("48 outstanding actions");
    await userEvent.click(screen.getByRole("button", { name: MORE }));

    await screen.findByText("Second thing");
    expect(screen.getByText("48 outstanding actions")).toBeTruthy();
    expect(screen.queryByText("1 outstanding action")).toBeNull();
  });

  // A refused SECOND page is not a refused day. The rows already loaded are
  // still true, and replacing them with an error panel throws away the work
  // the reader was in the middle of.
  it("keeps the loaded rows when the next page fails", async () => {
    let asked = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (!url.includes("/worklist")) {
          return new Response(JSON.stringify({ data: [] }), { status: 200 });
        }
        asked += 1;
        if (asked > 1) {
          return new Response(JSON.stringify({ code: "service_unavailable" }), {
            status: 503,
            headers: { "content-type": "application/problem+json" },
          });
        }
        return new Response(
          JSON.stringify(
            day({
              queue: [row({ id: "a", title: "Already read" })],
              summary: {
                urgent: 0,
                due: 1,
                in_play: 0,
                lower_priority: 0,
                total: 1,
              },
              next_cursor: "page-2",
            }),
          ),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }),
    );
    renderWorklist();

    await screen.findByText("Already read");
    await userEvent.click(screen.getByRole("button", { name: MORE }));

    // The row survives, and the failure is stated where it happened.
    await screen.findByText(en["scrollSentinel.failed"]);
    expect(
      screen.getByRole("button", { name: en["common.retry"] }),
    ).toBeTruthy();
    expect(screen.getByText("Already read")).toBeTruthy();
  });
});

describe("the end of a view", () => {
  it("says the view is wholly loaded once no cursor is left", async () => {
    stub(day({ queue: [row({ id: "a", title: "Only thing" })] }));
    renderWorklist();

    await screen.findByText("Only thing");
    expect(screen.getByText(en["worklist.end.today"])).toBeTruthy();
  });
});

describe("the header states the view's own figure", () => {
  it("counts only the view's categories, never upcoming work", async () => {
    stub(
      day({
        queue: [row({ id: "a" })],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 3 },
        counts: [
          { category: "tasks", considered: 3, shown: 1, more_available: false },
          {
            category: "decisions",
            considered: 5,
            shown: 0,
            more_available: false,
          },
          {
            category: "upcoming",
            considered: 4,
            shown: 0,
            more_available: false,
          },
        ],
      }),
    );
    renderWorklist();

    await screen.findByText("3 outstanding actions");
  });

  it("marks a figure the server could not count whole as a floor", async () => {
    stub(
      day({
        queue: [row({ id: "a" })],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 12 },
        counts: [
          { category: "tasks", considered: 12, shown: 1, more_available: true },
        ],
      }),
    );
    renderWorklist();

    await screen.findByText("12+ outstanding actions");
  });

  it.each([
    ["en", en],
    ["de", de],
    ["vi", viCatalog],
  ])("has a headline for every view in %s", (_, catalog) => {
    for (const view of ["today", "decisions", "system"] as const) {
      expect(catalog[`worklist.view.total.${view}_other`]).toContain("{count}");
    }
  });
});
