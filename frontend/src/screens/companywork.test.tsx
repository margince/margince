/** @vitest-environment jsdom */
import { render as rtlRender, cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyWorkCard, hasWorkInFlight } from "./companywork";

// What the work card is FOR, as assertions: the account's work in flight, one
// line per piece of it, each line's reason drawn from that piece's own
// records. The two rules the card exists to keep are the two hardest to see in
// a screenshot — the header states counts and never a reason, and a section a
// reader may not see is never reported as a section with nothing in it.

type Organization360 = components["schemas"]["Organization360"];

const page = { has_more: false, next_cursor: null };

const org = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function view(overrides: Partial<Organization360> = {}): Organization360 {
  return {
    as_of: "2026-08-01T09:00:00Z",
    organization: org,
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

const deal = {
  deal_id: "d-1",
  name: "Fleet retrofit 2026",
  status: "open",
  stage_name: "Proposal",
  amount: { amount_minor: 4_800_000, currency: "EUR" },
  stalled: false,
} as const;

const project = {
  project_id: "pr-1",
  name: "Depot fit-out",
  key: "DEP-12",
  phase: "delivering",
  quiet: false,
} as const;

function draw(three60: Organization360) {
  rtlRender(
    <LocaleProvider initial="en">
      <CompanyWorkCard view={three60} onOpenRecord={() => {}} />
    </LocaleProvider>,
  );
}

// What the header says BESIDE the card's own title: the count, and the
// assertions below about what may not join it. Scoped past the title because
// the title is a fixed label and the question here is what the card computes.
function headerSaid(): string {
  const head = document.querySelector(".panel-head");
  if (!head) {
    throw new Error("the work card drew no header");
  }
  const title = head.querySelector("h2");
  return (head.textContent ?? "").replace(title?.textContent ?? "", "");
}

afterEach(cleanup);

describe("the account's work in flight", () => {
  it("lists a deal and a project under their own subheads", () => {
    draw(view({ deals: { data: [deal], page, won_lifetime: { amount_minor: 0, currency: "EUR" }, lost_count: 0 }, projects: [project] }));

    expect(screen.getByRole("heading", { name: "Deals" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Projects" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Fleet retrofit 2026" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "DEP-12" })).toBeTruthy();
  });

  it("names who owes what, by when, from the deal's own overdue task", () => {
    draw(
      view({
        deals: {
          data: [
            {
              ...deal,
              attention: {
                kind: "overdue_task",
                title: "Send the retrofit quote",
                who: "Ida Keller",
                due_at: "2026-07-02T09:00:00Z",
              },
            },
          ],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
      }),
    );

    expect(
      screen.getByText(
        /Ida Keller was supposed to ‘Send the retrofit quote’ by 02\/07\/2026 and has not\./,
      ),
    ).toBeTruthy();
  });

  it("writes the overdue sentence without a name when the assignee is not this reader's to see", () => {
    draw(
      view({
        deals: {
          data: [
            {
              ...deal,
              attention: {
                kind: "overdue_task",
                title: "Send the retrofit quote",
                who: null,
                due_at: "2026-07-02T09:00:00Z",
              },
            },
          ],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
      }),
    );

    // The sentence stands on its own rather than leaving a gap where a name
    // would go, and no stray "null" reaches the screen.
    expect(
      screen.getByText(/‘Send the retrofit quote’ was due 02\/07\/2026 and is still open\./),
    ).toBeTruthy();
    expect(screen.queryByText(/null/)).toBeNull();
  });

  it("quotes a commitment rather than asserting over it", () => {
    draw(
      view({
        projects: [
          {
            ...project,
            attention: {
              kind: "commitment_theirs",
              title: "we'll confirm the depot slot once facilities sign off",
              who: "Ida Keller",
              due_at: null,
              source_activity_id: "a-1",
            },
          },
        ],
      }),
    );

    // The body is a proposition, not a noun phrase. It reads as reported
    // speech in quotes; "owes us we'll confirm…" is what a template built
    // around the body as an object would have produced.
    expect(
      screen.getByText(
        /Ida Keller said: ‘we'll confirm the depot slot once facilities sign off’/,
      ),
    ).toBeTruthy();
  });

  it("shows the stall only when there is no reason that explains it", () => {
    draw(
      view({
        deals: {
          data: [
            { ...deal, stalled: true },
            {
              ...deal,
              deal_id: "d-2",
              name: "Depot pilot",
              stalled: true,
              attention: {
                kind: "overdue_task",
                title: "Chase the pilot scope",
                who: "Ida Keller",
                due_at: "2026-07-02T09:00:00Z",
              },
            },
          ],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
      }),
    );

    // One clause per row: an overdue task IS a reason, a stall is the absence
    // of one, so the second row says the task rather than both.
    expect(screen.getAllByText(/last 60 days/)).toHaveLength(1);
    expect(screen.getByText(/Chase the pilot scope/)).toBeTruthy();
  });
});

describe("what the work card's header may say", () => {
  it("counts the work and states no reason", () => {
    draw(
      view({
        deals: {
          data: [
            {
              ...deal,
              attention: {
                kind: "overdue_task",
                title: "Send the retrofit quote",
                who: "Ida Keller",
                due_at: "2026-07-02T09:00:00Z",
              },
            },
          ],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
        projects: [project],
      }),
    );

    expect(headerSaid()).toContain("2 in flight");
    // The header is counts and facts. A summary sentence here would be the
    // blended account narrative this card replaced — one story written over
    // two engagements, which is the defect, not the layout.
    expect(headerSaid()).not.toContain("Ida Keller");
    expect(headerSaid()).not.toContain("Send the retrofit quote");
  });

  it("reads the count as a floor when a section was cut short", () => {
    draw(
      view({
        deals: {
          data: [deal],
          page: { has_more: true, next_cursor: null },
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
      }),
    );

    expect(headerSaid()).toContain("1+ in flight");
  });

  it("states no count at all when a section was withheld", () => {
    draw(
      view({
        deals: {
          data: [deal],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
        projects: undefined,
        projects_page: undefined,
        sections_omitted: ["projects"],
      }),
    );

    // "1 in flight" to a reader who can see the deals and not the projects is
    // a false statement about the account, not a partial one.
    expect(headerSaid()).not.toMatch(/in flight/);
  });
});

describe("a half of the card the reader may not have", () => {
  it("withholds that group and still draws the other", () => {
    draw(
      view({
        deals: {
          data: [deal],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
        projects: undefined,
        projects_page: undefined,
        sections_omitted: ["projects"],
      }),
    );

    expect(screen.getByRole("button", { name: "Fleet retrofit 2026" })).toBeTruthy();
    expect(screen.getByText("Hidden — your role cannot read this")).toBeTruthy();
    // Never the empty state: "no projects in flight" is a claim about the
    // account that this reader's payload does not support.
    expect(screen.queryByText("No projects in flight.")).toBeNull();
  });

  it("says the statuses are incomplete rather than letting bare rows read as settled", () => {
    draw(
      view({
        deals: {
          data: [deal],
          page,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
        },
        attention_withheld: true,
      }),
    );

    expect(screen.getByRole("button", { name: "Fleet retrofit 2026" })).toBeTruthy();
    expect(
      screen.getByText(
        "You cannot read this account’s conversations, so the rows above carry no reasons.",
      ),
    ).toBeTruthy();
  });
});

describe("whether the card holds the lead slot at all", () => {
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
});
