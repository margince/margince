/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { StateStrip } from "./company360";
import type { CompanyTab } from "./companytab";

// The company record's readings row draws FOUR cards on every account, and every
// one of them draws even when it has no reading — which is the rule this file
// exists for. A card that returns null leaves the row shorter by one and the
// reader unable to tell WHICH reading went missing; only an empty state is
// allowed to say there is none.
//
// It mounts StateStrip directly rather than through CompanyScreen: the count and
// the wording of an absent reading are the component's own contract, and reading
// them through the page would mean the fixture had to satisfy a dozen other
// cards to prove anything about this row.

type Organization360 = components["schemas"]["Organization360"];
type StateStripSection = components["schemas"]["Organization360StateStrip"];
type FinanceSummary = components["schemas"]["OrganizationFinanceSummary"];

const NO_CONNECTION: FinanceSummary = {
  organization_id: "o-1",
  state: "no_connection",
};

const organization: components["schemas"]["Organization"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function view(overrides: Partial<Organization360> = {}): Organization360 {
  return {
    as_of: "2026-08-18T09:00:00Z",
    organization,
    sections_omitted: [],
    ...overrides,
  };
}

// The finance summary is a query of its own, so even a row that asks nothing of
// it needs the route answered: an unstubbed fetch is a rejected promise, and a
// money slot reading "could not be read" for that reason would pass a test
// written about a connection that is simply not set up.
function stubFinance(summary: FinanceSummary | undefined, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/finance-summary")) {
        return new Response(JSON.stringify(summary ?? NO_CONNECTION), {
          status,
          headers: { "content-type": "application/json" },
        });
      }
      throw new Error(`the strip asked for ${pathname}, which no test stubs`);
    }),
  );
}

// Unmount between tests. Two mounted strips make `findByRole` ambiguous, and the
// failure it reports ("found multiple elements") looks nothing like the leak
// that caused it.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The real caller (organizations.tsx's CompanyBand) hands the strip its copy
// functions, so the fixtures do too — a strip fed identity functions would draw
// the wire enum and prove nothing about what a reader sees.
function renderStrip(
  three60: Organization360,
  onOpenTab?: (tab: CompanyTab) => void,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <StateStrip orgId="o-1" view={three60} onOpenTab={onOpenTab} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function readings() {
  const region = await screen.findByRole("region", {
    name: "Where this account stands",
  });
  const plate = within(region).getByTestId("company-strip");
  return { region, plate };
}

const prospect: StateStripSection = {
  account: { lifecycle: "prospect", relationship_types: [] },
  commercial: {
    open_count: 1,
    stalled_count: 0,
    priced_count: 1,
    converted_count: 0,
    open_pipeline_minor_base: 100_000,
    base_currency: "EUR",
    next_close_on: "2026-09-30",
  },
};

const customer: StateStripSection = {
  ...prospect,
  account: { lifecycle: "customer", relationship_types: ["customer"] },
};

describe("the company readings row is the shared strip, not a copy of it", () => {
  it("draws the design system's cards and none of its own", async () => {
    stubFinance(NO_CONNECTION);
    const { container } = renderStrip(view({ state_strip: prospect }));
    const { plate } = await readings();

    // Each card by its own class, which is what says the row draws the shared
    // StatCard primitive rather than a bespoke tile of its own.
    for (const slot of plate.children) {
      expect(slot.classList.contains("stat-card")).toBe(true);
    }
    expect(container.querySelector(".co-strip")).toBeNull();
  });

  // Four on every account: the verdict card, the pipeline card, the money
  // card and the relationship card, none of them conditional on the lifecycle.
  it("carries four cards for a prospect and four for a customer", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: prospect }));
    expect((await readings()).plate.childElementCount).toBe(4);

    cleanup();
    renderStrip(view({ state_strip: customer }));
    expect((await readings()).plate.childElementCount).toBe(4);
  });

  // The money card is on every account now, not only a customer's — what
  // changes with the lifecycle is what it has to say: a prospect has never
  // been billed, and a customer gets the real reading (or the reason there is
  // none).
  it("reads a money figure only once the account is a customer", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: prospect }));
    const asProspect = await readings();
    expect(
      within(asProspect.plate).getByText("Not a customer yet"),
    ).toBeTruthy();

    cleanup();
    renderStrip(view({ state_strip: customer }));
    const asCustomer = await readings();
    expect(
      await within(asCustomer.plate).findByText("Connect your accounting"),
    ).toBeTruthy();
    expect(
      within(asCustomer.plate).queryByText("Not a customer yet"),
    ).toBeNull();
  });
});

describe("a slot with no reading says which absence it is", () => {
  // An account nobody has worked: the deal grant is held, so the readings are
  // facts about the ACCOUNT rather than about the reader.
  const bare: StateStripSection = {
    account: { lifecycle: "prospect", relationship_types: [] },
    commercial: {
      open_count: 0,
      stalled_count: 0,
      priced_count: 0,
      converted_count: 0,
    },
  };

  it("still draws four cards when nothing has a figure", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: bare }));
    const { plate } = await readings();

    expect(plate.childElementCount).toBe(4);
    // Every card, labelled and answered. A blank card in the row reads as a
    // reading that failed to load rather than one the account does not have.
    for (const slot of plate.children) {
      expect(slot.querySelector(".stat-card-label")?.textContent).toBeTruthy();
      expect(slot.querySelector(".stat-card-value")?.textContent).toBeTruthy();
    }
  });

  it("names an unrated health reading rather than dropping the card", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: bare }));
    const { plate } = await readings();

    // Two different absences and two different words. Nothing rated is a
    // denominator ("0 of 3 rated"), and no open deal is an answer.
    expect(within(plate).getByText("Health")).toBeTruthy();
    expect(within(plate).getByText("0 of 3 rated")).toBeTruthy();
    expect(within(plate).getByText("No open deals")).toBeTruthy();
    // And no verdict borrowed from nowhere: "at risk" is a rating, and the
    // account has none.
    expect(plate.textContent).not.toMatch(/At risk|Good|Strong/);
  });

  // The half that must never be confused with the half above: a withheld
  // section is a fact about the READER, and reporting it as the account's own
  // standing is the business conclusion a rep would act on.
  it("says a withheld reading is withheld, never that there is none", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(
      view({
        state_strip: {
          account: { lifecycle: "prospect", relationship_types: [] },
          commercial: null,
        },
        sections_omitted: ["health"],
      }),
    );
    const { plate } = await readings();

    expect(plate.childElementCount).toBe(4);
    // Three of the four read from a section this caller may not see — the
    // pipeline card from the deal grant, the verdict and relationship cards
    // from the health grant — so all three say so, and none of them says the
    // account has no deals, no correspondence or no verdict. The money card
    // does not read from the health grant at all: on a non-customer it still
    // gives its own ordinary answer.
    expect(within(plate).getAllByText("Not shown").length).toBe(3);
    expect(within(plate).getByText("Not a customer yet")).toBeTruthy();
    expect(within(plate).queryByText("No open deals")).toBeNull();
    expect(plate.textContent).not.toMatch(/never written/i);
    // A withheld health section has no denominator either: "0 of 3 rated" would
    // be a count of what this reader is allowed to see, dressed as a count of
    // what has been judged.
    expect(within(plate).queryByText("0 of 3 rated")).toBeNull();
  });
});

describe("a reading offers the tab it is a reading of", () => {
  it("sends the reader to deals, finance and people from their own cards", async () => {
    stubFinance(NO_CONNECTION);
    const opened: string[] = [];
    renderStrip(view({ state_strip: customer }), (tab) => opened.push(tab));
    await readings();

    for (const name of ["Open deals", "Open finance", "Open people"]) {
      await userEvent.click(screen.getByRole("button", { name }));
    }

    expect(opened).toEqual(["deals", "finance", "people"]);
  });

  // A strip drawn where there is no tab strip to send anybody to — the
  // storybook, a mirrored workspace — must not draw a door onto nothing.
  it("draws no door when the caller has nowhere to send them", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: customer }));
    await readings();

    expect(screen.queryByRole("button", { name: "Open deals" })).toBeNull();
  });
});
