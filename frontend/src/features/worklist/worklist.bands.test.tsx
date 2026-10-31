// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "../../testing/worklist.testkit";

// The headings, and the band that holds nothing.
//
// `bands` has been on the wire since the field was added — every band in draw
// order, including one with no rows — and no client file read it. Headings were
// inferred from the rows instead, which draws the same page in every case but
// one: a band with nothing under it has no row to hang a heading on, so it drew
// nothing at all. A reader whose Now band was empty saw a page that started at
// Prospecting and could not tell that from a day where Now had simply not
// been read.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The band headings, by role rather than by text. "Review" is also a filter
// pill and a row verb, so a text match finds three elements and reports an
// ambiguity where the page is drawing exactly what it should.
function headings(): (string | null)[] {
  return screen
    .getAllByRole("heading", { level: 3 })
    .map((node) => node.textContent);
}

// A day carrying the server's own band list. The order here is deliberately
// the server's draw order, which is what the page must follow rather than the
// order the rows happen to arrive in.
function banded(over: Partial<Parameters<typeof day>[0]> = {}) {
  return day({
    bands: [
      { band: "now", shown: 0 },
      { band: "build_pipeline", shown: 0 },
      { band: "keep_momentum", shown: 1 },
      { band: "review", shown: 0 },
    ],
    queue: [
      row({
        id: "drifting",
        title: "Send the retrofit quote",
        band: "keep_momentum",
      }),
    ],
    summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
    ...over,
  });
}

describe("an empty band draws nothing", () => {
  // The queue says once that the day is clear. A heading and a line per empty
  // band repeated that under every band, which is the stack this removes.
  it("draws neither a heading nor a line for an empty band", async () => {
    stub(banded());
    renderWorklist("en");

    expect(await screen.findByText("Send the retrofit quote")).toBeTruthy();
    expect(headings()).not.toContain("Now");
    expect(headings()).not.toContain("Prospecting");
    expect(document.querySelector(".worklist-band-clear")).toBeNull();
  });

  // A band that holds rows draws them under its heading.
  it("draws a band that holds rows", async () => {
    stub(banded());
    renderWorklist("en");

    expect(await screen.findByText("Send the retrofit quote")).toBeTruthy();
    expect(headings()).toContain("Keep momentum");
  });
});

describe("the headings follow the QUEUE", () => {
  // The page never re-sorts. The server sends the rows already ordered with
  // each band contiguous, and the rank numbers are positions in THAT order — so
  // a page that walked `bands` and collected each one's rows would reorder them
  // whenever the two disagreed, and print rank 2 above rank 1.
  //
  // This fixture is that disagreement: the declared order says now-then-review
  // and the queue arrives review-first. The queue wins, and the ranks stay 1, 2.
  it("keeps the queue's order when the declared order disagrees", async () => {
    stub(
      day({
        bands: [
          { band: "now", shown: 1 },
          { band: "review", shown: 1 },
        ],
        queue: [
          row({ id: "first", title: "A duplicate to judge", band: "review" }),
          row({ id: "second", title: "A buyer waiting", band: "now" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );
    renderWorklist("en");

    await screen.findByText("A buyer waiting");
    const drawn = headings();
    expect(drawn.indexOf("Review")).toBeLessThan(drawn.indexOf("Now"));
    // And the ranks still ascend down the page, which is the defect this rule
    // exists to prevent rather than a second assertion about the same thing.
    expect(
      [...document.querySelectorAll(".worklist-rank")].map(
        (n) => n.textContent,
      ),
    ).toEqual(["1", "2"]);
  });
});

describe("a server that sends no bands still draws headings", () => {
  // `bands` is optional in the contract, so a client talking to an older server
  // gets none. The page falls back to the bands its rows name — which is the
  // behaviour that shipped before this file, and is right as far as it goes.
  it("falls back to the bands the rows name", async () => {
    stub(
      day({
        queue: [
          row({ id: "one", title: "A buyer waiting", band: "now" }),
          row({ id: "two", title: "A duplicate to judge", band: "review" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );
    renderWorklist("en");

    await screen.findByText("A buyer waiting");
    expect(headings()).toContain("Now");
    expect(headings()).toContain("Review");
    // And no line claiming a band is clear: the server said nothing about the
    // bands it is not sending, so the page must not answer for it.
    expect(
      screen.queryByText("No urgent items. Remaining work is below."),
    ).toBeNull();
  });
});

describe("the rank runs across the headings", () => {
  // The number is the page's central claim about the order, so it counts the
  // whole queue rather than each heading's run. Splitting the flat list into
  // one <ol> per band is exactly the change that would restart it, and a page
  // drawing two number ones tells a reader the order means nothing.
  it("numbers rows 1..n over the whole queue, not per band", async () => {
    stub(
      day({
        bands: [
          { band: "now", shown: 1 },
          { band: "review", shown: 2 },
        ],
        queue: [
          row({ id: "first", title: "A buyer waiting", band: "now" }),
          row({ id: "second", title: "A duplicate to judge", band: "review" }),
          row({ id: "third", title: "Another to judge", band: "review" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 2, total: 3 },
      }),
    );
    renderWorklist("en");

    await screen.findByText("A buyer waiting");
    const ranks = [...document.querySelectorAll(".worklist-rank")].map(
      (node) => node.textContent,
    );
    expect(ranks).toEqual(["1", "2", "3"]);
  });
});

// Work due later reads as later.
//
// The lane now carries what is coming as well as what is owed, and without a
// sub-heading tomorrow's deadline sits in the same run as today's — a reader
// scanning the Now band would count work they do not owe yet.
describe("the day a run of work is due", () => {
  it("heads the runs due later and leaves today's under the band", async () => {
    stub(
      banded({
        bands: [
          { band: "now", shown: 4 },
          { band: "build_pipeline", shown: 0 },
          { band: "keep_momentum", shown: 0 },
          { band: "review", shown: 0 },
        ],
        queue: [
          row({
            id: "a",
            band: "now",
            due_group: "overdue",
            title: "Overdue one",
          }),
          row({
            id: "b",
            band: "now",
            due_group: "today",
            title: "Answer the buyer",
          }),
          row({
            id: "c",
            band: "now",
            due_group: "tomorrow",
            title: "Call the architect",
          }),
          row({
            id: "d",
            band: "now",
            due_group: "this_week",
            title: "Send the revised quote",
          }),
        ],
        summary: { urgent: 0, due: 4, lower_priority: 0, total: 4 },
      }),
    );
    renderWorklist("en");
    await screen.findByText("Call the architect");

    const subHeadings = screen
      .getAllByRole("heading", { level: 4 })
      .map((node) => node.textContent);
    expect(subHeadings).toEqual(["Due tomorrow", "Due this week"]);

    // The rows themselves all still draw: a grouping that hides work would be
    // a worse failure than one that labels it wrongly.
    for (const title of [
      "Overdue one",
      "Answer the buyer",
      "Call the architect",
      "Send the revised quote",
    ]) {
      expect(screen.getByText(title)).toBeTruthy();
    }
  });

  it("draws no sub-heading at all when everything is due today", async () => {
    stub(
      banded({
        bands: [
          { band: "now", shown: 2 },
          { band: "build_pipeline", shown: 0 },
          { band: "keep_momentum", shown: 0 },
          { band: "review", shown: 0 },
        ],
        queue: [
          row({
            id: "a",
            band: "now",
            due_group: "overdue",
            title: "Overdue one",
          }),
          row({
            id: "b",
            band: "now",
            due_group: "today",
            title: "Answer the buyer",
          }),
        ],
        summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
      }),
    );
    renderWorklist("en");
    await screen.findByText("Overdue one");

    expect(screen.queryAllByRole("heading", { level: 4 })).toHaveLength(0);
  });
});
