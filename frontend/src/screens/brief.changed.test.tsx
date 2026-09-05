/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ChangedSinceBrief } from "./brief.changed";
import type { Worklist, WorklistItem } from "./worklist.queries";

// What has happened since the night looked.
//
// The cases are all about the difference between three states the wire keeps
// apart and a careless client would not: changed, not changed, and no run to
// compare against.

afterEach(cleanup);

function item(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "i1",
    source: "waiting_customer",
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    actions: ["open"],
    ...over,
  } as unknown as WorklistItem;
}

function draw(queue: WorklistItem[]) {
  return render(
    <LocaleProvider initial="en">
      <ChangedSinceBrief
        day={{ queue, as_of: "2026-09-03T06:42:00Z" } as unknown as Worklist}
      />
    </LocaleProvider>,
  );
}

describe("what changed since the brief", () => {
  it("names the rows the server marked as new", () => {
    draw([
      item({ id: "a", title: "Aster replied", changed_since_brief: true }),
      item({
        id: "b",
        title: "Fleet retrofit moved",
        changed_since_brief: true,
      }),
      item({ id: "c", title: "Old news", changed_since_brief: false }),
    ]);

    expect(screen.getByText(/Aster replied/)).toBeTruthy();
    expect(screen.getByText(/Fleet retrofit moved/)).toBeTruthy();
    expect(screen.queryByText(/Old news/)).toBeNull();
  });

  // THE distinction the whole strip turns on. A row carries no flag when there
  // was no run to compare against, and "the night saw this" is a different fact
  // from "there was no night". Reading absent as false would report a calm
  // morning on the one day nothing could be known about it.
  it("draws nothing when there was no run to compare against", () => {
    const { container } = draw([
      item({ id: "a", title: "Aster replied" }),
      item({ id: "b", title: "Fleet retrofit moved" }),
    ]);

    expect(container.firstChild).toBeNull();
  });

  it("draws nothing on a morning the night already saw whole", () => {
    const { container } = draw([
      item({ id: "a", changed_since_brief: false }),
      item({ id: "b", changed_since_brief: false }),
    ]);

    expect(container.firstChild).toBeNull();
  });

  // Three names and then a count. A strip that listed eleven titles would be a
  // second queue above the queue.
  it("names three and counts the rest", () => {
    draw(
      Array.from({ length: 6 }, (_, at) =>
        item({
          id: `row-${at}`,
          title: `Row ${at}`,
          changed_since_brief: true,
        }),
      ),
    );

    expect(screen.getByText(/Row 0/)).toBeTruthy();
    expect(screen.getByText(/Row 2/)).toBeTruthy();
    expect(screen.queryByText(/Row 3/)).toBeNull();
    // A substring match rather than a regex: the copy starts with "+", which a
    // regex reads as a quantifier over nothing.
    expect(
      screen.getByText((text) =>
        text.includes(en["brief.changed.more"].replace("{count}", "3")),
      ),
    ).toBeTruthy();
  });

  it("says nothing about a remainder when it named them all", () => {
    draw([
      item({ id: "a", title: "Aster replied", changed_since_brief: true }),
    ]);

    expect(screen.queryByText(/more/)).toBeNull();
  });

  // The decisions deck above answers approvals, and this strip must not name
  // one as well: the reader would see the same decision twice, once as a card
  // they can answer and once as a line pointing at a worklist where it is not
  // the row they came for. brief.sentence.ts holds the ONE spelling of what
  // this page is answerable for, and reading the raw queue here was that rule
  // reappearing one element higher.
  it("leaves the decisions the deck above already answers to the deck", () => {
    const { container } = draw([
      item({
        id: "a",
        source: "approval",
        title: "Approve the discount",
        changed_since_brief: true,
      }),
      item({ id: "b", title: "Aster replied", changed_since_brief: true }),
    ]);

    expect(container.textContent).not.toContain("Approve the discount");
    expect(screen.getByText(/Aster replied/)).toBeTruthy();
  });

  // THE mixed state, and the case that earns the `=== true` filter beyond the
  // all-absent one. A run happened, so some rows carry the flag — but a row with
  // no material timestamp of its own carries none even then
  // (attention/briefdedupe.go skips it), and reading that absence as "changed"
  // would name a row the night saw perfectly well.
  it("names only the flagged rows when a run left some rows unflagged", () => {
    draw([
      item({ id: "a", title: "Aster replied", changed_since_brief: true }),
      item({ id: "b", title: "Undateable row" }),
      item({ id: "c", title: "Seen last night", changed_since_brief: false }),
    ]);

    expect(screen.getByText(/Aster replied/)).toBeTruthy();
    expect(screen.queryByText(/Undateable row/)).toBeNull();
    expect(screen.queryByText(/Seen last night/)).toBeNull();
  });

  // The only way out of the strip. Without it a reader is told three things
  // moved and given nowhere to go and see them.
  it("offers the way to the rows it named", () => {
    draw([
      item({ id: "a", title: "Aster replied", changed_since_brief: true }),
    ]);

    const link = screen.getByText(en["brief.changed.open"]);
    expect(link.getAttribute("href")).toBe("#/worklist");
  });

  // A payload with no queue at all draws nothing rather than throwing: a page
  // that throws is a worse answer than one that draws nothing.
  it("draws nothing rather than throwing on an answer with no queue", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <ChangedSinceBrief day={{} as unknown as Worklist} />
      </LocaleProvider>,
    );

    expect(container.firstChild).toBeNull();
  });
});
