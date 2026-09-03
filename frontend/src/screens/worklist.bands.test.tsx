// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// The headings, and the band that holds nothing.
//
// `bands` has been on the wire since the field was added — every band in draw
// order, including one with no rows — and no client file read it. Headings were
// inferred from the rows instead, which draws the same page in every case but
// one: a band with nothing under it has no row to hang a heading on, so it drew
// nothing at all. A reader whose Now band was empty saw a page that started at
// Build pipeline and could not tell that from a day where Now had simply not
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

describe("a band holding nothing says so", () => {
  // The case the field exists for. Nothing is waiting on this reader today and
  // the page says it in as many words, rather than leaving the reader to infer
  // it from a heading that is not there.
  it("draws the empty band's own line", async () => {
    stub(banded());
    renderWorklist("en");

    expect(await screen.findByText("Nothing needs you today.")).toBeTruthy();
    expect(screen.getByText("Nothing to review.")).toBeTruthy();
  });

  // And the heading above it, so the line is attributed. A line saying
  // "Nothing needs you today" under no heading is a sentence about the whole
  // page rather than about one band.
  it("draws the empty band's heading with it", async () => {
    stub(banded());
    renderWorklist("en");

    await screen.findByText("Nothing needs you today.");
    expect(headings()).toContain("Now");
    expect(headings()).toContain("Review");
  });

  // Each band says what the reader is clear OF. Four copies of one generic
  // "nothing here" would cost the same space and tell the reader less.
  it("says something different about each empty band", async () => {
    stub(banded());
    renderWorklist("en");

    await screen.findByText("Nothing needs you today.");
    for (const line of [
      "No new pipeline work waiting.",
      "Nothing to review.",
    ]) {
      expect(screen.queryByText(line)).toBeTruthy();
    }
    // And NOT the line for the band that holds a row.
    expect(screen.queryByText("Nothing agreed is drifting.")).toBeNull();
  });

  // A band that DOES hold rows draws them, not a line claiming it is clear.
  it("says nothing about a band that holds rows", async () => {
    stub(banded());
    renderWorklist("en");

    expect(await screen.findByText("Send the retrofit quote")).toBeTruthy();
    expect(headings()).toContain("Keep momentum");
  });
});

describe("a page with more to load claims no band is empty", () => {
  // The pagination trap. The queue arrives band-sorted, so a band missing from
  // page one may hold rows on page three — and "nothing needs you today" over
  // a Show more button is the page claiming to have looked at work it has not
  // fetched. The reassurance waits until the whole day is loaded.
  it("draws no empty-band line while a page is still unfetched", async () => {
    stub(banded({ next_cursor: "page-two" }));
    renderWorklist("en");

    // Anchored on a line drawn in the same pass, so the absence is asserted
    // against a rendered page rather than against one that has not arrived.
    expect(await screen.findByText("Send the retrofit quote")).toBeTruthy();
    expect(screen.queryByText("Nothing needs you today.")).toBeNull();
    expect(screen.queryByText("Nothing to review.")).toBeNull();
  });
});

describe("the headings follow the server's order", () => {
  // The page draws the bands in the order the SERVER listed them, not the order
  // its rows arrived in. Two rows whose bands are listed the other way round
  // are the case that tells the two rules apart.
  it("draws a band before one the server listed after it", async () => {
    stub(
      day({
        bands: [
          { band: "now", shown: 1 },
          { band: "review", shown: 1 },
        ],
        queue: [
          row({ id: "later", title: "A duplicate to judge", band: "review" }),
          row({ id: "first", title: "A buyer waiting", band: "now" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );
    renderWorklist("en");

    await screen.findByText("A buyer waiting");
    const drawn = headings();
    expect(drawn.indexOf("Now")).toBeLessThan(drawn.indexOf("Review"));
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
    expect(screen.queryByText("Nothing needs you today.")).toBeNull();
  });
});
