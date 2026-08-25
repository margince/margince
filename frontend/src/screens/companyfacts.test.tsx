// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyFacts } from "./companyfacts";

// The facts box states three things and must never state a fourth by
// accident. Both readings it carries can be WITHHELD rather than empty, and
// the whole reason this file exists is that "withheld" and "none" are one
// pixel apart on screen and opposite facts about the account: one says the
// reader cannot see, the other says there is nothing to see.

type Organization360 = components["schemas"]["Organization360"];
type Organization = components["schemas"]["Organization"];

const page = { has_more: false, next_cursor: null };

const org: Organization = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
} as Organization;

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
    state_strip: {
      account: { lifecycle: "customer", relationship_types: [] },
      commercial: {
        open_count: 0,
        stalled_count: 0,
        priced_count: 0,
        converted_count: 0,
      },
    },
    ...overrides,
  } as unknown as Organization360;
}

// The owner control reads the roster through react-query, so the box needs a
// client. It is the only thing in here that fetches.
function draw(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(cleanup);

// Three states, not two. Rev 1 of this box collapsed the permission case into
// the empty one, which tells a reader who may not see deals that the account
// has none.
describe("what the open pipeline says", () => {
  it("names a figure when the deals are readable and priced", () => {
    draw(
      <CompanyFacts
        org={org}
        view={view({
          state_strip: {
            account: { lifecycle: "customer", relationship_types: [] },
            commercial: {
              open_count: 2,
              stalled_count: 0,
              priced_count: 2,
              converted_count: 0,
              open_pipeline_minor_base: 6_400_000,
              base_currency: "EUR",
            },
          },
        } as Partial<Organization360>)}
      />,
    );

    expect(screen.getByText(/64,000/)).toBeTruthy();
  });

  it("says the account has no open deals rather than showing nothing", () => {
    draw(<CompanyFacts org={org} view={view()} />);

    expect(screen.getByText("No open deals")).toBeTruthy();
  });

  // The strip itself is still present when the deal grant is missing, so its
  // `commercial` half going absent is a fact about the READER.
  it("says withheld, not 'no deals', when the commercial half is absent", () => {
    draw(
      <CompanyFacts
        org={org}
        view={view({
          state_strip: {
            account: { lifecycle: "customer", relationship_types: [] },
          },
        } as Partial<Organization360>)}
      />,
    );

    expect(
      screen.getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();
    expect(screen.queryByText("No open deals")).toBeNull();
  });

  // Deals that exist and nobody has costed. A dash here would read as "we do
  // not know what they are worth", which is the one reading it is not.
  it("says the pipeline is unpriced rather than printing a zero", () => {
    draw(
      <CompanyFacts
        org={org}
        view={view({
          state_strip: {
            account: { lifecycle: "customer", relationship_types: [] },
            commercial: {
              open_count: 3,
              stalled_count: 0,
              priced_count: 0,
              converted_count: 0,
            },
          },
        } as Partial<Organization360>)}
      />,
    );

    expect(screen.getByText("Not priced yet")).toBeTruthy();
    expect(screen.queryByText(/€0/)).toBeNull();
  });
});

describe("what the in-flight count says", () => {
  it("counts the open deals and the live projects", () => {
    draw(
      <CompanyFacts
        org={org}
        view={view({
          deals: {
            data: [{ deal_id: "d-1" }, { deal_id: "d-2" }],
            page,
            won_lifetime: { amount_minor: 0, currency: "EUR" },
            lost_count: 0,
          },
          projects: [
            { project_id: "p-1", name: "Rollout", phase: "delivering" },
            { project_id: "p-2", name: "Done", phase: "closed" },
          ],
        } as unknown as Partial<Organization360>)}
      />,
    );

    // The closed project is not in flight, so this is 2 and 1, never 2 and 2.
    expect(screen.getByText(/2 deals/)).toBeTruthy();
    // Singular, not "1 projects": each half pluralises on its own count.
    expect(screen.getByText(/1 project(?!s)/)).toBeTruthy();
  });

  // A count that folded an unreadable half into it would be a false statement
  // rather than a partial one: "1 project" on an account whose deals the
  // reader cannot see reads as an account with no deals.
  it("gives no count at all when either half is withheld", () => {
    draw(
      <CompanyFacts
        org={org}
        view={view({
          projects: undefined,
          sections_omitted: ["projects"],
        } as unknown as Partial<Organization360>)}
      />,
    );

    expect(screen.queryByText(/deals ·/)).toBeNull();
    expect(
      screen.getAllByText("Hidden — your role cannot read this").length,
    ).toBeGreaterThan(0);
  });

  it("says nothing is in flight when both halves are readable and empty", () => {
    draw(<CompanyFacts org={org} view={view()} />);

    expect(screen.getByText("Nothing")).toBeTruthy();
  });

  // The sections are capped, so a count taken off the rows is a floor. Saying
  // "2 deals" on an account with forty is a number the account does not have.
  it("marks the count as a floor when a section was cut short", () => {
    draw(
      <CompanyFacts
        org={org}
        view={view({
          deals: {
            data: [{ deal_id: "d-1" }],
            page: { has_more: true, next_cursor: "c" },
            won_lifetime: { amount_minor: 0, currency: "EUR" },
            lost_count: 0,
          },
        } as unknown as Partial<Organization360>)}
      />,
    );

    expect(screen.getByText(/or more/)).toBeTruthy();
  });
});

// The 360 is still in flight. "Reading…" is a different answer from both
// "withheld" and "none", and drawing either of those over a read that has not
// answered states a fact the page does not have yet.
describe("before the composite has answered", () => {
  it("says it is still reading rather than reporting an absence", () => {
    draw(<CompanyFacts org={org} view={undefined} />);

    expect(screen.getAllByText("Reading…").length).toBe(2);
    expect(screen.queryByText("No open deals")).toBeNull();
    expect(screen.queryByText("Nothing")).toBeNull();
  });
});
