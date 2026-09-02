/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { focusOf } from "./worklist.focus";
import { NextUp, nextUpOf } from "./worklist.nextup";

// The short answer to "and then?".
//
// Two things make this list worth having rather than a second queue: it never
// repeats the row the focus card already took, and it never offers a row that
// card would have refused. Both are about the page agreeing with itself.

type WorklistItem = components["schemas"]["WorklistItem"];

function item(id: string, over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id,
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: `Row ${id}`,
    because: [],
    actions: ["act"],
    primary_action: "act",
    band: "now",
    subject: { type: "person", id: `person-${id}`, label: `Person ${id}` },
    ...over,
  };
}

afterEach(cleanup);

describe("the Next-up list", () => {
  // The focus card lifts the first row OUT. A list that then re-offered it
  // would tell the reader their next two jobs are the same job.
  it("never repeats the row the focus card took", () => {
    const queue = [item("a"), item("b"), item("c")];
    const focused = focusOf(queue);

    const next = nextUpOf(queue, focused);

    expect(focused?.id).toBe("a");
    expect(next.map((row) => row.id)).toEqual(["b", "c"]);
  });

  // One admission rule, two surfaces. A review row is judgement the queue
  // collects to be worked in one pass, and offering it as "and then" tells a
  // rep that hygiene is their morning.
  it("offers nothing the focus card would have refused", () => {
    const queue = [
      item("a"),
      item("hygiene", { band: "review", category: "decisions" }),
      item("noverb", { primary_action: undefined, actions: [] }),
      item("b"),
    ];

    const next = nextUpOf(queue, focusOf(queue));

    expect(next.map((row) => row.id)).toEqual(["b"]);
  });

  // `id` is the OWNING RECORD's id, not the row's — the contract says so, and
  // the queue keys its rows on source+id because of it. A person with an
  // unanswered message and an overdue task appears twice under one id, so
  // excluding the focused row by id would silently drop the second row too.
  it("drops only the focused row when one record raises two", () => {
    const queue = [
      item("person-1"),
      item("person-1", { source: "task", category: "tasks", level: 4 }),
      item("other"),
    ];
    const focused = focusOf(queue);

    const next = nextUpOf(queue, focused);

    expect(next.length).toBe(2);
    expect(next[0]?.source).toBe("task");
    expect(next[1]?.id).toBe("other");
  });

  // Bounded, or it is the queue again in a smaller box. A reader has to be able
  // to see the end of it.
  it("stays finite however long the day is", () => {
    const queue = Array.from({ length: 40 }, (_, i) => item(`row-${i}`));

    const next = nextUpOf(queue, focusOf(queue));

    expect(next.length).toBe(3);
  });

  // The server ranked these rows; the list re-ranks nothing. Reordering here
  // would be a second answer to what matters most today.
  it("keeps the server's order", () => {
    const queue = [item("a"), item("b"), item("c"), item("d")];

    const next = nextUpOf(queue, focusOf(queue));

    expect(next.map((row) => row.id)).toEqual(["b", "c", "d"]);
  });

  // A day whose only actionable row is the focused one has no "and then". An
  // empty panel there would report a finished morning as a broken component.
  it("draws nothing when there is nothing after the focused row", () => {
    render(
      <LocaleProvider initial="en">
        <NextUp items={[]} />
      </LocaleProvider>,
    );

    expect(screen.queryByText("And then")).toBeNull();
  });

  // Through `itemTitle`, so the list names a row exactly as the queue and the
  // card do — including the record's own name beside a title that does not
  // carry it. Three rows reading "Follow up with the new lead" cannot be told
  // apart, which is the defect that rule exists for.
  it("names each row the way the rest of the page names it", () => {
    const rows = [item("b"), item("c")];
    render(
      <LocaleProvider initial="en">
        <NextUp items={rows} />
      </LocaleProvider>,
    );

    expect(screen.getByText("And then")).toBeTruthy();
    expect(screen.getByText("Row b · Person b")).toBeTruthy();
    expect(screen.getByText("Row c · Person c")).toBeTruthy();
  });
});
