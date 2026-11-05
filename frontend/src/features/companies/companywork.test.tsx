/** @vitest-environment happy-dom */
import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { hasWorkInFlight } from "./companywork";

// Whether the account has any work in flight at all. That question decides
// between the account's own reading and the growth-fit panel in the same slot.

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
    company,
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
