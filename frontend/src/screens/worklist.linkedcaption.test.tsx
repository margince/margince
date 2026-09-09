/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { WorklistHeader } from "./worklist.header";
import type { Worklist, WorklistFilter } from "./worklist.queries";

// What a page narrowed BY LINK says about itself.
//
// The pill row holds the seven kinds of work and `all`. A link-only narrowing
// presses none of them, so without a sentence the reader arrives from Brief at a
// short queue with the whole row unselected and nothing saying why — which reads
// as a broken page rather than a narrowed one. A unit test over the vocabulary
// cannot see that: it passes whether or not anything is drawn.

afterEach(cleanup);

function day(): Worklist {
  return {
    as_of: "2026-09-02T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, in_play: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      revenue_at_risk_minor: 0,
      revenue_currency: "EUR",
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
  } as unknown as Worklist;
}

function draw(filter: WorklistFilter, onFilter = () => {}) {
  return render(
    <LocaleProvider initial="en">
      <WorklistHeader
        day={day()}
        loaded={0}
        scope="mine"
        filter={filter}
        owner=""
        onScope={() => {}}
        onFilter={onFilter}
        onOwner={() => {}}
      />
    </LocaleProvider>,
  );
}

describe("a page narrowed by link", () => {
  it("names the narrowing the reader arrived under", () => {
    draw("changed_since_brief");

    expect(
      screen.getByText(en["worklist.filter.linked.changed_since_brief"], {
        exact: false,
      }),
    ).toBeTruthy();
  });

  it("names the feed's cut too", () => {
    draw("except_decisions");

    expect(
      screen.getByText(en["worklist.filter.linked.except_decisions"], {
        exact: false,
      }),
    ).toBeTruthy();
  });

  // The way back out. A narrowing with no pill pressed leaves the reader no
  // control that undoes it, so the sentence carries one.
  it("offers a way back to the whole queue", () => {
    const onFilter = vi.fn();
    draw("changed_since_brief", onFilter);

    screen.getByText(en["worklist.filter.linked.clear"]).click();

    expect(onFilter).toHaveBeenCalledWith("all");
  });

  // The positive control. Without it every assertion above would pass against a
  // header that drew the caption on EVERY page, including the seven where a
  // pressed pill already says which cut is on screen.
  it("says nothing on a page the pill row can name", () => {
    draw("meetings");

    expect(screen.queryByText(en["worklist.filter.linked.clear"])).toBeNull();
  });
});
