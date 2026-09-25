// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi as viCatalog } from "../i18n/vi";
import {
  day,
  panelNamed,
  renderWorklist,
  row,
  stub,
  stubWalk,
} from "./worklist.testkit";

const MORE = en["worklist.more"];

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

  // The header describes the assembled DAY, the queue describes what is
  // loaded. Reading the figures off the latest page instead would make the
  // summary shrink as the reader pages further in.
  //
  // Every figure comes off the FIRST page's `summary`, which the server counts
  // over the whole day rather than over the rows it cut. The fixture says so:
  // its summary is far bigger than either page's row count, and it agrees with
  // `counts[].considered` — which is what makes it one sentence about one
  // population rather than four figures about the page beside a fifth about the
  // day. A fixture whose bands could be a count of its own page would pass
  // whichever the header meant, which is how this shipped.
  it("AC-WORKLIST-SDR-04: keeps the day's figures still while the rows grow", async () => {
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
        summary: {
          urgent: 3,
          due: 9,
          in_play: 2,
          lower_priority: 6,
          total: 48,
        },
        counts,
        next_cursor: "page-2",
      }),
      day({
        queue: [row({ id: "b", title: "Second thing" })],
        // The later page's own figures, which must NOT reach the header.
        summary: { urgent: 0, due: 0, in_play: 0, lower_priority: 0, total: 1 },
        counts,
      }),
    ]);
    renderWorklist();

    await screen.findByText(/3 urgent/);
    await userEvent.click(screen.getByRole("button", { name: MORE }));

    await screen.findByText("Second thing");
    await waitFor(() => {
      expect(screen.getByText(/3 urgent/)).toBeTruthy();
    });
    // One scope, stated: the total the sentence ends on is the same population
    // the bands are counted over, and neither is this page's single row.
    expect(screen.getByText(/· 48 total/)).toBeTruthy();
    expect(screen.queryByText(/· 1 total/)).toBeNull();
  });

  // A server that does not send `in_play` has not said there is none of it.
  // Printing 0 for silence is the under-reporting this line exists to prevent.
  it("says nothing about a middle band the server did not count", async () => {
    stub(
      day({
        queue: [row({ id: "a", title: "Only thing" })],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
        counts: [
          {
            category: "tasks" as const,
            considered: 1,
            shown: 1,
            more_available: false,
          },
        ],
      }),
    );
    renderWorklist();

    await screen.findByText(/1 urgent/);
    expect(screen.queryByText(/in play/)).toBeNull();
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
          return new Response(JSON.stringify({ code: "unavailable" }), {
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
    await screen.findByText(/More items did not load/);
    expect(screen.getByText("Already read")).toBeTruthy();
  });
});

// A next page's rows land in whichever panel their destination names, so the
// control that asks for it belongs to neither.
describe("the way to the rest of the day speaks for both panels", () => {
  const judgement = row({
    id: "pair-1",
    source: "dedupe_candidate",
    destination: "review",
    title: "Two records for one company",
  });

  it("stands below To review, outside both panels", async () => {
    stub(
      day({
        queue: [
          row({ id: "t1", destination: "today", title: "A task" }),
          judgement,
        ],
        next_cursor: "page-2",
      }),
    );
    renderWorklist();

    const today = panelNamed(await screen.findByText(en["worklist.queue"]));
    const review = panelNamed(screen.getByText(en["worklist.review"]));
    const more = screen.getByRole("button", { name: MORE });
    expect(within(today).queryByRole("button", { name: MORE })).toBeNull();
    expect(within(review).queryByRole("button", { name: MORE })).toBeNull();
    expect(
      review.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
    expect(more).toHaveAccessibleDescription(
      "More of the day: new items can land in “Today” or “To review”.",
    );
  });

  it("lands a page of only review rows in To review and stays in reach", async () => {
    stubWalk([
      day({
        queue: [row({ id: "t1", destination: "today", title: "A task" })],
        next_cursor: "page-2",
      }),
      day({ queue: [judgement], next_cursor: "page-3" }),
    ]);
    renderWorklist();
    const user = userEvent.setup();

    await screen.findByText("A task");
    await user.click(screen.getByRole("button", { name: MORE }));

    const review = panelNamed(await screen.findByText(en["worklist.review"]));
    expect(
      within(review).getByText("Two records for one company"),
    ).toBeTruthy();
    const more = screen.getByRole("button", { name: MORE });
    expect(
      review.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
  });
});

describe("the header states the split", () => {
  it("names the two panels' halves, a zero half included", async () => {
    stub(
      day({
        queue: [row({ id: "pair-1", destination: "review" })],
        summary: {
          urgent: 0,
          due: 0,
          lower_priority: 5,
          total: 5,
          buckets: { urgent: 0, due_today: 0, planned: 0, review: 5 },
        },
      }),
    );
    renderWorklist();

    await screen.findByText("0 today · 5 to review");
    // No bare total: it would count both panels as one.
    expect(screen.queryByText(/\d total/)).toBeNull();
  });

  it("keeps the five-figure sentence for a server without the partition", async () => {
    stub(
      day({
        queue: [row({ id: "a" })],
        summary: { urgent: 1, due: 2, in_play: 0, lower_priority: 3, total: 6 },
      }),
    );
    renderWorklist();

    await screen.findByText(
      "1 urgent · 2 due · 0 in play · 3 routine · 6 total",
    );
    expect(screen.queryByText(/to review/)).toBeNull();
  });

  // Today is all three of its parts at once: the bands under it break it down.
  it("counts every part of Today in its half", async () => {
    stub(
      day({
        queue: [row({ id: "a" })],
        summary: {
          urgent: 1,
          due: 2,
          lower_priority: 3,
          total: 10,
          buckets: { urgent: 1, due_today: 2, planned: 3, review: 4 },
        },
      }),
    );
    renderWorklist();

    await screen.findByText("6 today · 4 to review");
  });

  // Like the five-figure sentence, the split is the DAY's and is read off page
  // one: a later page's partition describes only its own slice.
  it("keeps the split still while the rows grow", async () => {
    stubWalk([
      day({
        queue: [row({ id: "a", title: "First thing" })],
        summary: {
          urgent: 1,
          due: 2,
          lower_priority: 6,
          total: 14,
          buckets: { urgent: 1, due_today: 2, planned: 6, review: 5 },
        },
        next_cursor: "page-2",
      }),
      day({
        queue: [row({ id: "b", title: "Second thing" })],
        summary: {
          urgent: 0,
          due: 0,
          lower_priority: 1,
          total: 1,
          buckets: { urgent: 0, due_today: 0, planned: 1, review: 0 },
        },
      }),
    ]);
    renderWorklist();
    const user = userEvent.setup();

    await screen.findByText("9 today · 5 to review");
    await user.click(screen.getByRole("button", { name: MORE }));

    await screen.findByText("Second thing");
    expect(screen.getByText("9 today · 5 to review")).toBeTruthy();
    expect(screen.queryByText("1 today · 0 to review")).toBeNull();
  });

  // Two names for one panel is how a reader stops trusting the header, so each
  // half is its panel's heading, in every locale.
  it.each([
    ["en", en],
    ["de", de],
    ["vi", viCatalog],
  ])("names both halves with the %s panel headings", (_, catalog) => {
    expect(catalog["worklist.summary.split"]).toBe(
      `{today} ${catalog["worklist.queue"].toLowerCase()} · {review} ${catalog["worklist.review"].toLowerCase()}`,
    );
  });
});
