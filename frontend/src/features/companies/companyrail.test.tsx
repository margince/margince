/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  emptyPage,
  renderCards,
  renderRail,
  stub,
  view,
} from "./companyrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function contact(
  id: string,
  name: string,
  score: number,
  over: Record<string, unknown> = {},
) {
  return {
    contact_id: id,
    full_name: name,
    title: "Head of Fleet",
    primary_email: `${id}@brandt.example`,
    strength: { score, bucket: "moderate", factors: {} },
    deal_roles: [],
    consent: {},
    ...over,
  };
}

function deal(id: string, name: string, over: Record<string, unknown> = {}) {
  return {
    deal_id: id,
    name,
    status: "open",
    stage_name: "Proposal",
    amount: { amount_minor: 4_800_000, currency: "EUR" },
    expected_close_date: "2026-09-30",
    stalled: false,
    ...over,
  };
}

function deals(rows: unknown[]) {
  return {
    data: rows,
    page: emptyPage,
    won_lifetime: { amount_minor: 0, currency: null },
    lost_count: 0,
  };
}

// The section a card's heading opens.
function card(name: string): HTMLElement {
  const section = screen.getByRole("heading", { name }).closest("section");
  if (!section) {
    throw new Error(`no card named ${name}`);
  }
  return section;
}

describe("the Overview's Contacts card", () => {
  it("draws the two strongest contacts as cards with their seat and address", () => {
    stub();
    renderCards({
      view: view({
        contacts: {
          data: [
            contact("p-1", "Ravi Weak", 10),
            contact("p-2", "Dana Buyer", 80, {
              deal_roles: [{ deal_id: "d-1", role: "champion" }],
            }),
            contact("p-3", "Lena Middle", 40),
          ],
          page: emptyPage,
        },
      }),
    });
    const contacts = within(card("Contacts"));
    expect(contacts.getByRole("link", { name: "Dana Buyer" })).toHaveAttribute(
      "href",
      "/#/contacts/p-2",
    );
    expect(contacts.getByText("Lena Middle")).toBeInTheDocument();
    expect(contacts.queryByText("Ravi Weak")).toBeNull();
    expect(contacts.getByText("Champion")).toBeInTheDocument();
    expect(
      contacts.getAllByText("Head of Fleet · Brandt Automotive GmbH").length,
    ).toBe(2);
  });

  it("stacks its cards as the card list, which keeps a gap between them", () => {
    stub();
    renderCards({
      view: view({
        contacts: {
          data: [
            contact("p-1", "Ravi One", 80),
            contact("p-2", "Dana Two", 40),
          ],
          page: emptyPage,
        },
      }),
    });
    const list = within(card("Contacts")).getByRole("list");
    expect(list).toHaveClass("record-card-list");
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
  });

  it("says no economic buyer is named while a deal is open and nobody holds the seat", () => {
    stub();
    renderCards({
      view: view({
        contacts: { data: [contact("p-1", "Dana Buyer", 80)], page: emptyPage },
        deals: deals([deal("d-1", "Fleet retrofit")]),
      }),
    });
    expect(
      within(card("Contacts")).getByText("No economic buyer named yet"),
    ).toBeInTheDocument();
  });

  it("says nothing about the seat once somebody holds it", () => {
    stub();
    renderCards({
      view: view({
        contacts: {
          data: [
            contact("p-1", "Dana Buyer", 80, {
              deal_roles: [{ deal_id: "d-1", role: "economic_buyer" }],
            }),
          ],
          page: emptyPage,
        },
        deals: deals([deal("d-1", "Fleet retrofit")]),
      }),
    });
    expect(screen.queryByText("No economic buyer named yet")).toBeNull();
  });

  it("names the first known seat and draws no pill for a role it does not know", () => {
    stub();
    renderCards({
      view: view({
        contacts: {
          data: [
            contact("p-1", "Dana Buyer", 80, {
              deal_roles: [
                { deal_id: "d-1", role: "toString" },
                { deal_id: "d-2", role: "blocker" },
              ],
            }),
            contact("p-2", "Lena Middle", 40, {
              deal_roles: [
                { deal_id: "d-1", role: "constructor" },
                { deal_id: "d-2", role: "gatekeeper" },
              ],
            }),
          ],
          page: emptyPage,
        },
      }),
    });
    const contacts = card("Contacts");
    expect(within(contacts).getByText("Blocker")).toBeInTheDocument();
    // A prototype name found by a bare index would print the function's source.
    expect(contacts.textContent).not.toMatch(
      /function|toString|constructor|gatekeeper/,
    );
  });
});

describe("the Overview's Deals and projects card", () => {
  it("lists open deals, then projects, with the amount in whole units", () => {
    stub();
    renderCards({
      view: view({
        deals: deals([
          deal("d-1", "Fleet retrofit"),
          deal("d-2", "Phase one licence", { status: "won" }),
        ]),
        projects: [
          {
            project_id: "pr-1",
            name: "Depot rollout",
            phase: "delivering",
            target_end_date: "2026-11-30",
            quiet: false,
          },
        ],
      }),
    });
    const work = within(card("Deals and projects"));
    const rows = work.getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Fleet retrofit");
    expect(rows[0]).toHaveTextContent("€48,000");
    expect(rows[0]).toHaveTextContent("Proposal · close 30 Sep");
    expect(rows[1]).toHaveTextContent("Depot rollout");
    expect(rows[1]).toHaveTextContent("Project · Delivering · ends 30 Nov");
    expect(work.queryByText("Phase one licence")).toBeNull();
  });

  it("says nothing is open on an account with no live work", () => {
    stub();
    renderCards();
    expect(
      within(card("Deals and projects")).getByText("No open deal or project"),
    ).toBeInTheDocument();
  });

  it("opens the Deals and projects tab from its link", async () => {
    stub();
    const onTab = vi.fn();
    renderCards({ onTab });
    await userEvent.click(
      within(card("Deals and projects")).getByRole("button", {
        name: "All deals",
      }),
    );
    expect(onTab).toHaveBeenCalledWith("deals");
  });
});

describe("the Overview's Also on your list card", () => {
  const step = (id: string, over: Record<string, unknown> = {}) => ({
    activity_id: id,
    subject: `Task ${id}`,
    due_at: null,
    overdue: false,
    ...over,
  });

  it("marks each open task by where it stands at the read's own day", () => {
    stub();
    renderCards({
      view: view({
        as_of: "2026-08-13T09:00:00Z",
        next_steps: {
          data: [
            step("t-1", { due_at: "2026-07-25T09:00:00Z", overdue: true }),
            step("t-2", { due_at: "2026-08-19T09:00:00Z" }),
            step("t-3"),
          ],
          page: emptyPage,
        },
      }),
    });
    const list = within(card("Also on your list"));
    const rows = list.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("19 days late");
    expect(rows[1]).toHaveTextContent("In 6 days");
    expect(rows[2]).toHaveTextContent("No date");
  });
});

describe("the details' Way in row", () => {
  it("draws the strongest route as the contact's card", () => {
    stub();
    renderRail({
      view: view({
        strength: {
          score: 80,
          bucket: "strong",
          contributor_contact_id: "p-2",
          contact_count: 1,
        },
        contacts: { data: [contact("p-2", "Dana Buyer", 80)], page: emptyPage },
      }),
    });
    expect(screen.getByText("Way in")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Dana Buyer" })).toHaveAttribute(
      "href",
      "/#/contacts/p-2",
    );
  });
});
