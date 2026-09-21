/** @vitest-environment happy-dom */
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

type Company360 = components["schemas"]["Company360"];
type StateStripSection = components["schemas"]["Company360StateStrip"];
type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];

const NO_CONNECTION: FinanceSummary = {
  company_id: "o-1",
  state: "no_connection",
};

const company: components["schemas"]["Company"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function view(overrides: Partial<Company360> = {}): Company360 {
  return {
    as_of: "2026-08-18T09:00:00Z",
    company,
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

// The real caller (companies.tsx's CompanyBand) hands the strip its copy
// functions, so the fixtures do too — a strip fed identity functions would draw
// the wire enum and prove nothing about what a reader sees.
function renderStrip(
  three60: Company360,
  onOpenTab?: (tab: CompanyTab) => void,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <StateStrip companyId="o-1" view={three60} onOpenTab={onOpenTab} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function readings() {
  const region = await screen.findByRole("region", {
    name: "Where this account stands",
  });
  // The row IS the region: the shared strip carries the name and the test id
  // on one element, so its children are the doors.
  expect(region.dataset.testid).toBe("company-strip");
  return { region, plate: region };
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
  it("carries five doors for a prospect and five for a customer", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: prospect }));
    expect((await readings()).plate.childElementCount).toBe(5);

    cleanup();
    renderStrip(view({ state_strip: customer }));
    expect((await readings()).plate.childElementCount).toBe(5);
  });

  // The money card is on every account now, not only a customer's — what
  // changes with the lifecycle is what it has to say: a prospect has never
  // been billed, and a customer gets the real reading (or the reason there is
  // none).
  it("reads a money figure only once the account is a customer", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: prospect }));
    const asProspect = await readings();
    // The stage itself under the word, so "Not invoiced" on a prospect and
    // "Not invoiced" on a customer we have never billed stay apart.
    expect(within(asProspect.plate).getByText("Prospect")).toBeTruthy();
    expect(within(asProspect.plate).getByText("Not invoiced")).toBeTruthy();

    cleanup();
    renderStrip(view({ state_strip: customer }));
    const asCustomer = await readings();
    expect(
      await within(asCustomer.plate).findByText("Accounting not connected"),
    ).toBeTruthy();
    expect(within(asCustomer.plate).queryByText("Prospect")).toBeNull();
  });

  // The twelve-month window is a fact about invoices, not about the stage the
  // account stands in today. A former customer billed for three years reading
  // "Not invoiced" was the lifecycle answering a question about money.
  it("reads a former customer's money the way a current one's is read", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(
      view({
        state_strip: {
          ...prospect,
          account: {
            lifecycle: "former_customer",
            relationship_types: ["customer"],
          },
        },
      }),
    );
    const { plate } = await readings();

    expect(
      await within(plate).findByText("Accounting not connected"),
    ).toBeTruthy();
    expect(within(plate).queryByText("Former customer")).toBeNull();
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

  it("still draws five doors when nothing has a figure", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: bare }));
    const { plate } = await readings();

    expect(plate.childElementCount).toBe(5);
    // Every door, labelled and answered. A blank card in the row reads as a
    // reading that failed to load rather than one the account does not have.
    for (const slot of plate.children) {
      expect(slot.querySelector(".stat-card-label")?.textContent).toBeTruthy();
      expect(slot.querySelector(".stat-card-value")?.textContent).toBeTruthy();
    }
  });

  it("names each reading it has none of, and states no verdict", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: bare }));
    const { plate } = await readings();

    // Four different absences and four different words: no open deal, no
    // relationship rated, no word exchanged, nothing on the calendar.
    expect(within(plate).getAllByText("None").length).toBe(2);
    expect(within(plate).getByText("Not assessed")).toBeTruthy();
    expect(within(plate).getByText("None scheduled")).toBeTruthy();
    // And no verdict borrowed from nowhere: the account's standing is the
    // 360's word under this row, never a sixth door here.
    expect(plate.textContent).not.toMatch(/At risk|Good|Strong|Health/);
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

    expect(plate.childElementCount).toBe(5);
    // Two of the five read from a section this caller may not see — the
    // pipeline from the deal grant, the conversation from the health grant —
    // so both say so, and neither says the account has no deals or no
    // correspondence. The last touch and the calendar do not read from those
    // grants at all and give their own ordinary answers, as does the money on
    // a non-customer.
    expect(within(plate).getAllByText("Restricted").length).toBe(2);
    expect(within(plate).getByText("Prospect")).toBeTruthy();
    expect(within(plate).getByText("None")).toBeTruthy();
    expect(within(plate).getByText("None scheduled")).toBeTruthy();
    expect(plate.textContent).not.toMatch(/No exchange|Unanswered/i);
    expect(within(plate).queryByText("Not assessed")).toBeNull();
  });
});

// The relationship slot with no inbound word to read. "They have never
// written" is one sentence over two opposite accounts: one nobody has ever
// spoken to, and one being ignored. The second is the one a rep acts on, and
// for a year it was hidden behind the first.
describe("silence on the relationship slot names which silence it is", () => {
  const spoken: StateStripSection = {
    account: { lifecycle: "prospect", relationship_types: [] },
    commercial: null,
  };

  function relationship(plate: HTMLElement): HTMLElement {
    const card = within(plate).getByText("Relationship").closest(".stat-card");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the relationship reading has no card");
    }
    return card;
  }

  it("says there is no exchange when nothing was ever sent either", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(
      view({ state_strip: spoken, health: { days_since_last_inbound: null } }),
    );
    const card = relationship((await readings()).plate);

    expect(within(card).getByText("No exchange")).toBeTruthy();
    expect(card.querySelector(".stat-card-detail")).toBeNull();
  });

  // The days run from the last outbound to the view's own `as_of`, never from
  // the reader's clock: the card must not age a day while the page sits open.
  it("says unanswered, and for how long, when we wrote and they did not", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(
      view({
        state_strip: spoken,
        health: { days_since_last_inbound: null },
        last_outbound_at: "2026-08-08T09:00:00Z",
      }),
    );
    const card = relationship((await readings()).plate);

    expect(within(card).getByText("Unanswered")).toBeTruthy();
    expect(within(card).getByText("No reply · 10 d")).toBeTruthy();
  });

  // A quiet relationship says how long nothing has come back, in the same
  // words the unanswered slot uses — one claim about one relationship. A share
  // of the exchange here would describe a conversation that has stopped, so it
  // is not stated even when the reading carries one.
  it("says how long a quiet relationship has gone unanswered", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(
      view({
        state_strip: spoken,
        health: { days_since_last_inbound: 45, reply_balance: 0.2 },
      }),
    );
    const card = relationship((await readings()).plate);

    expect(within(card).getByText("Quiet")).toBeTruthy();
    expect(within(card).getByText("No reply · 45 d")).toBeTruthy();
    expect(card.textContent).not.toMatch(/inbound/i);
  });

  // The share belongs to the relationships that are still running: there the
  // dates say nothing a reader can act on and the balance does.
  it("keeps the share of the exchange for a live relationship", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(
      view({
        state_strip: spoken,
        health: { days_since_last_inbound: 3, reply_balance: 0.2 },
      }),
    );
    const card = relationship((await readings()).plate);

    expect(within(card).getByText("One-sided")).toBeTruthy();
    expect(within(card).getByText("20% inbound")).toBeTruthy();
  });
});

// A card's door, by the reading it belongs to. Every door on the plate carries
// the SAME word — "Open" is the component's, not the caller's — so what tells
// five of them apart for a screen reader is the DESCRIPTION, which is the
// reading's own label. Folded into the name it read "Open Open deals".
function door(label: string): HTMLElement {
  return screen.getByRole("button", { name: "Open", description: label });
}

describe("a reading offers the tab it is a reading of", () => {
  it("sends the reader to deals, finance and history from their own doors", async () => {
    stubFinance(NO_CONNECTION);
    const opened: string[] = [];
    renderStrip(view({ state_strip: customer }), (tab) => opened.push(tab));
    await readings();

    for (const label of ["Open deals", "Revenue · 12 mo"]) {
      await userEvent.click(door(label));
    }
    // THREE readings open the same page, and each is read off it: the
    // conversation, the last touch and the next meeting all live on the
    // history. The next-meeting card used to open the task list instead,
    // which made it the strip's only route to tasks — and made it a card
    // that said one thing and did another. Tasks is reached from the tab
    // strip; a meeting card is not the place to hide the door to it.
    for (const label of ["Relationship", "Last contact", "Next meeting"]) {
      await userEvent.click(door(label));
    }

    expect(opened).toEqual([
      "deals",
      "finance",
      "timeline",
      "timeline",
      "timeline",
    ]);
  });

  // A strip drawn where there is no tab strip to send anybody to — the
  // storybook, a mirrored workspace — must not draw a door onto nothing.
  it("draws no door when the caller has nowhere to send them", async () => {
    stubFinance(NO_CONNECTION);
    renderStrip(view({ state_strip: customer }));
    await readings();

    expect(screen.queryAllByRole("button", { name: /^Open / })).toHaveLength(0);
  });
});
