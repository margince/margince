/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { hasWorkInFlight, sinceLastVisitFooter } from "./companywork";

// What this file is FOR, as assertions: whether the account's own reading
// still has anything to say since the reader's last visit, and whether the
// account has any work in flight at all, the question that decides between
// the account's own reading and the growth-fit panel in the same slot. The
// account's open deals themselves are drawn on the Deals tab and the rail's
// own DealsSection now, not by a card in this file.

type Company360 = components["schemas"]["Company360"];

const page = { has_more: false, next_cursor: null };

const company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function view(overrides: Partial<Company360> = {}): Company360 {
  return {
    as_of: "2026-08-01T09:00:00Z",
    company: company,
    sections_omitted: [],
    deals: {
      data: [],
      page,
      won_lifetime: { amount_minor: 0, currency: "EUR" },
      lost_count: 0,
    },
    projects: [],
    projects_page: page,
    ...overrides,
  };
}

const project = {
  project_id: "pr-1",
  name: "Depot fit-out",
  key: "DEP-12",
  phase: "delivering",
  quiet: false,
} as const;

afterEach(cleanup);

describe("what moved since the reader was last here", () => {
  // What a footer slot consumes: Panel draws its band on truthiness, so the
  // ONLY safe answer for "nothing to report" is undefined. An element that
  // renders null is still truthy and costs the record a blank row.
  it("hands a footer slot nothing at all when nothing moved", () => {
    const quiet = view({
      since_last_visit: {
        baseline_at: "2026-07-01T09:00:00Z",
        new_activities: 0,
        deal_stage_moves: 0,
        pending_proposals: 0,
      },
    });
    expect(sinceLastVisitFooter(quiet)).toBeUndefined();
  });

  it("greets a first visit", () => {
    const first = view({
      since_last_visit: {
        baseline_at: null,
        new_activities: 0,
        deal_stage_moves: 0,
        pending_proposals: 0,
      },
    });
    expect(sinceLastVisitFooter(first)).toBeDefined();
    rtlRender(
      <LocaleProvider initial="en">
        {sinceLastVisitFooter(first)}
      </LocaleProvider>,
    );
    expect(
      screen.getByText("You are opening this account for the first time."),
    ).toBeTruthy();
  });

  it("counts what landed", () => {
    const moved = view({
      since_last_visit: {
        baseline_at: "2026-07-01T09:00:00Z",
        new_activities: 3,
        deal_stage_moves: 0,
        pending_proposals: 0,
      },
    });
    rtlRender(
      <LocaleProvider initial="en">
        {sinceLastVisitFooter(moved)}
      </LocaleProvider>,
    );
    expect(screen.getByText("3 new items since your last visit.")).toBeTruthy();
  });

  it("speaks for a withheld section even when nothing moved", () => {
    expect(
      sinceLastVisitFooter(view({ sections_omitted: ["deals"] })),
    ).toBeDefined();
  });

  // A composite that answered WITHOUT the omission list at all. The field is
  // required on the wire, so every fixture in this file supplies one and none
  // of them covered this: a payload missing it once threw, and the whole
  // company record rendered as "this view no longer works". An older server,
  // a trimmed mock and a partial cache entry all produce this payload.
  it("says nothing is withheld when the payload carries no omission list", () => {
    const noList = view();
    // Deleted rather than set to undefined: absent and explicitly-undefined are
    // the same bytes over the wire, and `delete` is the one that reproduces a
    // response that never named the field.
    delete (noList as { sections_omitted?: unknown }).sections_omitted;

    expect(() => sinceLastVisitFooter(noList)).not.toThrow();
    expect(sinceLastVisitFooter(noList)).toBeUndefined();
  });
});

describe("whether the account holds the work slot at all", () => {
  it("yields it when both halves are readable and empty", () => {
    expect(hasWorkInFlight(view())).toBe(false);
  });

  it("keeps it while any work is in flight", () => {
    expect(hasWorkInFlight(view({ projects: [project] }))).toBe(true);
  });

  it("keeps it when a half was withheld, because nobody said the account has none", () => {
    // The growth-fit panel in this slot asks "should we sell to them at all",
    // which to a reader who simply may not see the pipeline reads as "there
    // is no pipeline here".
    expect(
      hasWorkInFlight(
        view({
          deals: undefined,
          sections_omitted: ["deals"],
        }),
      ),
    ).toBe(true);
  });

  it("yields it for a closed project, which is history rather than work", () => {
    expect(
      hasWorkInFlight(view({ projects: [{ ...project, phase: "closed" }] })),
    ).toBe(false);
  });

  // The contract marks `sections_omitted` required, and this was once the only
  // reader in the tree that believed it: every other one guards the field. A
  // payload without it took the WHOLE account page down through the error
  // boundary, because this runs before the page decides which card the slot
  // gets. A read that names no omissions withholds nothing, which is the safe
  // reading: "there is none" understates rather than inventing work.
  it("survives a payload that names no omissions at all", () => {
    const partial = view();
    // Through `Reflect`, because the whole point is that the TYPE still says
    // the field is there while the payload does not carry it — `delete` on a
    // required property is a compile error, and asserting the type away to
    // reach it would be the very claim under test.
    Reflect.deleteProperty(partial, "sections_omitted");
    expect(() => hasWorkInFlight(partial)).not.toThrow();
    expect(hasWorkInFlight(partial)).toBe(false);
  });
});
