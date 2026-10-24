/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { MONEY_ABSENT } from "../../format/format";
import { LocaleProvider } from "../../i18n";
import type { CompanyNaming } from "./dealcompanymarks";
import { buildColumns, buildStageTotals, FxLine, OffersPanel } from "./deals";

type Stage = components["schemas"]["Stage"];
type Deal = components["schemas"]["Deal"];

const stages: Stage[] = [
  {
    id: "s1",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
];

// No company resolved and no company withheld: these cases are about money, so
// every card here draws no company row at all.
const noCompany: CompanyNaming = { marks: new Map(), unreadable: new Set() };

function deal(overrides: Partial<Deal>): Deal {
  return {
    id: "d1",
    name: "Fleet retrofit",
    amount_minor: 4_800_000,
    currency: "EUR",
    pipeline_id: "pl",
    stage_id: "s1",
    status: "open",
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}

describe("stage money the report did not state", () => {
  it("never invents a base currency from a native row", () => {
    const totals = buildStageTotals([
      { stage_id: "s1", currency: "USD", deals: 3, base_minor: 100 },
    ]);
    expect(totals.get("s1")).toMatchObject({
      count: 3,
      currency: null,
      baseMinor: null,
      weightedBaseMinor: null,
    });
  });
  it("does not turn a null converted sum into zero", () => {
    const totals = buildStageTotals(
      [
        {
          stage_id: "s1",
          deals: 2,
          base_minor: null,
          weighted_base_minor: null,
        },
      ],
      "EUR",
      "2026-10-01",
    );
    expect(totals.get("s1")).toMatchObject({
      currency: "EUR",
      baseMinor: null,
      weightedBaseMinor: null,
      unpricedCount: null,
    });
  });
});

describe("buildColumns with money the deal does not carry", () => {
  it("carries a currency-less stage total through to the column", () => {
    const totals = buildStageTotals([
      { stage_id: "s1", currency: null, deals: 2, base_minor: null },
    ]);
    const columns = buildColumns(stages, [], totals, noCompany);
    expect(columns[0].rawMinor).toBeNull();
    expect(columns[0].weightedMinor).toBeNull();
    expect(columns[0].currency).toBeNull();
    expect(columns[0].count).toBe(2);
  });

  // The card's own money, unfilled. An unpriced deal used to reach the board as
  // 0 EUR, which every reader takes for a priced deal worth nothing.
  it("a deal with neither half of its money reaches the card as absent, not as zero EUR", () => {
    const columns = buildColumns(
      stages,
      [deal({ amount_minor: null, currency: null })],
      new Map(),
      noCompany,
    );
    expect(columns[0].deals[0].valueMinor).toBeNull();
    expect(columns[0].deals[0].currency).toBeNull();
  });

  it("a deal with an amount and no currency keeps the amount and invents no currency", () => {
    const columns = buildColumns(
      stages,
      [deal({ amount_minor: 4_800_000, currency: null })],
      new Map(),
      noCompany,
    );
    expect(columns[0].deals[0].valueMinor).toBe(4_800_000);
    expect(columns[0].deals[0].currency).toBeNull();
  });
});

// A converted figure has to name the currency it was converted INTO, and that
// currency belongs to the installation, not to the code. Reading it as a
// constant put a euro sign over francs on every installation whose base is not
// the euro — the one error a conversion must not make, because the reader has
// no way to see it.
describe("the FX base line names the installation's base currency", () => {
  // Each case asserts over the whole document, so a left-over render from the
  // previous one turns a single match into an ambiguous one.
  afterEach(cleanup);

  const line = (baseCurrency: string | null, amountMinor: number | null) =>
    render(
      <LocaleProvider initial="en">
        <FxLine
          amountMinor={amountMinor}
          baseCurrency={baseCurrency}
          fxRateToBase="0.92"
          fxRateDate="2026-07-01"
          locale="en"
        />
      </LocaleProvider>,
    );

  it("states the base currency the installation reported", () => {
    line("CHF", 100_000);
    expect(screen.getByText(/CHF/)).toBeTruthy();
    expect(screen.queryByText(/€/)).toBeNull();
  });

  it("states no figure while the base currency is unknown", () => {
    line(null, 100_000);
    expect(screen.getByText(new RegExp(MONEY_ABSENT))).toBeTruthy();
    expect(screen.queryByText(/€/)).toBeNull();
  });

  it("converts an absent amount to nothing rather than to zero", () => {
    line("CHF", null);
    expect(screen.getByText(new RegExp(MONEY_ABSENT))).toBeTruthy();
    expect(screen.queryByText(/CHF\s*0/)).toBeNull();
  });
});

// An offer is written in the DEAL's currency, so a deal nobody has priced has
// nothing to write one in. The control stays visible and says why: creating the
// offer in a currency the code picked would write an invented denomination onto
// a record, which no reader can see and no later edit knows to question.
describe("an offer cannot be created in a currency nobody chose", () => {
  afterEach(cleanup);

  const panel = (
    dealCurrency: string | null,
    onCreate: (currency: string) => void = () => {},
  ) =>
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <OffersPanel
            offers={[]}
            creating={false}
            locale="en"
            dealCurrency={dealCurrency}
            onCreate={onCreate}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

  it("refuses the control on an unpriced deal, and points it at the reason", () => {
    panel(null);
    const button = screen.getByRole("button", { name: "New offer" });
    // Button's own refusal contract: `reason` disables the control and adds the
    // explanation to aria-describedby.
    // The native attribute rather than a jest-dom matcher: this suite registers
    // none, and the attribute IS Button's refusal contract.
    expect(button.hasAttribute("disabled")).toBe(true);
    const describedBy = button.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(
      document.getElementById((describedBy ?? "").split(" ")[0])?.textContent,
    ).toContain("Set the deal value first");
  });

  it("does not fire the write when refused", async () => {
    const created: string[] = [];
    panel(null, (currency: string) => created.push(currency));
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "New offer" }));
    expect(created).toEqual([]);
  });

  it("creates in the deal's own currency, never a substitute", async () => {
    const created: string[] = [];
    panel("VND", (currency: string) => created.push(currency));
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "New offer" }));
    expect(created).toEqual(["VND"]);
  });
});
