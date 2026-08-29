/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { PeopleCard } from "./company360";

// The committee-gap callout, which is a claim about a DEAL: this deal has nobody
// named as its champion.
//
// It used to be a claim about the account — the roles held anywhere across every
// open deal, unioned — and the union is what made it silent on the accounts that
// needed it most. Two deals, a champion on one and an economic buyer on the
// other, covered both roles between them and reported no gap while each deal was
// short.
//
// PeopleCard is mounted directly: the callout is its footer, and the account page
// reaches it through a composite read a stub would have to fake anyway.

type Organization = components["schemas"]["Organization"];
type Organization360 = components["schemas"]["Organization360"];
type Contact = NonNullable<Organization360["people"]>["data"][number];
type Deal = NonNullable<Organization360["deals"]>["data"][number];

const PAGE = { has_more: false, next_cursor: null };
const FACTORS = { recency: 0, frequency: 0, reciprocity: 0, direction: 0 };

const ORG: Organization = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function contact(personId: string, roles: Contact["deal_roles"]): Contact {
  return {
    person_id: personId,
    full_name: `Contact ${personId}`,
    strength: { score: 0, bucket: "none", factors: FACTORS },
    deal_roles: roles,
    consent: {},
  };
}

function deal(dealId: string, name: string): Deal {
  return { deal_id: dealId, name, status: "open", stalled: false };
}

function view(contacts: Contact[], deals: Deal[]): Organization360 {
  return {
    as_of: "2026-06-01T09:00:00Z",
    organization: ORG,
    sections_omitted: [],
    people: { data: contacts, page: PAGE },
    deals: {
      data: deals,
      page: PAGE,
      won_lifetime: { amount_minor: 0, currency: "EUR" },
      lost_count: 0,
    },
  };
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(cleanup);

describe("the committee gap a multi-deal account is short of", () => {
  it("names the deal each role is missing on, one line per deal", () => {
    render(
      <PeopleCard
        view={view(
          [
            contact("p-champ", [{ deal_id: "d-a", role: "champion" }]),
            contact("p-buyer", [{ deal_id: "d-b", role: "economic_buyer" }]),
          ],
          [deal("d-a", "Renewal"), deal("d-b", "New business")],
        )}
      />,
    );

    // Each deal is short of the role the OTHER one has. The union said nothing.
    expect(
      screen.getByText(
        "No economic buyer is named on Renewal yet — set one on the contact who is.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "No champion is named on New business yet — set one on the contact who is.",
      ),
    ).toBeTruthy();
  });

  it("leaves the deal name off when there is only one deal to be on", () => {
    // Same reason the row's own roles line omits it: on a single-deal account
    // the deal's name is a word the reader already has.
    render(
      <PeopleCard
        view={view([contact("p-1", [])], [deal("d-a", "Renewal")])}
      />,
    );

    expect(
      screen.getByText(
        "No champion / economic buyer is named on the open deal yet — set one on the contact who is.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/on Renewal yet/)).toBeNull();
  });

  it("counts every unfilled pair in the coverage line, not the role types", () => {
    render(
      <PeopleCard
        view={view(
          [contact("p-1", [])],
          [deal("d-a", "Renewal"), deal("d-b", "New business")],
        )}
      />,
    );

    // Two deals, both short of both roles. "2 role gaps" was the old reading —
    // the number of role TYPES, which cannot exceed two however many deals are
    // short of them.
    expect(screen.getByText(/4 role gaps/)).toBeTruthy();
  });

  it("says nothing at all about a deal whose committee is complete", () => {
    render(
      <PeopleCard
        view={view(
          [
            contact("p-1", [
              { deal_id: "d-a", role: "champion" },
              { deal_id: "d-a", role: "economic_buyer" },
            ]),
          ],
          [deal("d-a", "Renewal")],
        )}
      />,
    );

    expect(screen.queryByText(/is named on/)).toBeNull();
    expect(screen.queryByText(/role gaps/)).toBeNull();
  });
});
