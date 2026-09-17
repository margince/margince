/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./companies";
import { company360, companyBackstop, stubFetch } from "./company.fixtures";

// What an account is under contract for reads on the Deals tab, which is
// where the pipeline and the account's other work in flight already live.
// These cases are about the three states that are easy to draw dishonestly:
// no agreement at all, an agreement nobody priced, and one with no renewal
// date. None of the three may render as a zero or as a date.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

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

type Contracts = NonNullable<
  components["schemas"]["Company360StateStrip"]["contracts"]
>;

// The 360 as this account's strip carries it. `state_strip.contracts` is
// absent by default in the shared fixture — which is itself one of the states
// below, so a case that wants agreements says so explicitly.
function viewWith(contracts?: Contracts) {
  return {
    ...company360,
    state_strip: {
      account: { status: "customer", relationship_types: ["customer"] },
      contracts,
    },
  };
}

// The block's own element, found by the line that names it: everything the
// reading says about the account's agreements is inside it, so an assertion
// about what is NOT there cannot be satisfied by a figure belonging to some
// other part of the commercial panel.
function contractBlock(caption: string): HTMLElement {
  const element = screen.getByText(caption).parentElement;
  if (!element) {
    throw new Error(`the contract line "${caption}" sits in no block`);
  }
  return element;
}

async function openDealsTab() {
  await userEvent.click(await screen.findByRole("button", { name: /^Deals/ }));
}

describe("the Deals tab's commercial card reports the contract standing", () => {
  it("names the value and the renewal", async () => {
    stubFetch(companyBackstop, {
      company360: viewWith({
        active_count: 2,
        cancellation_pending: false,
        base_currency: "EUR",
        total_basis_value_minor_base: 30_000_000,
        annualized_value_minor_base: 12_000_000,
        nearest_renewal_on: "2027-03-01",
      }),
    });

    render(<CompanyScreen id="o-1" />);
    await openDealsTab();

    const block = await waitFor(() =>
      contractBlock("Under contract · 2 active"),
    );
    // The two bases stay apart: a three-year total and a per-year figure span
    // different periods, so the card must not summarize them into one number.
    expect(block.textContent).toContain("300,000");
    expect(block.textContent).toContain("120,000");
    expect(block.textContent).not.toContain("420,000");
    expect(block.textContent).toMatch(/Renews.*2027/);
  });

  it("draws no Commercial card on an account with no agreement", async () => {
    stubFetch(companyBackstop, {
      company360: viewWith({ active_count: 0, cancellation_pending: false }),
    });

    render(<CompanyScreen id="o-1" />);
    await openDealsTab();

    // An account with no agreement draws no Commercial card at all: not a
    // value of zero, which would read as an agreement worth nothing, and not
    // a card holding one line that says nothing is there.
    await screen.findByRole("heading", { name: "Deals" });
    expect(screen.queryByRole("heading", { name: "Commercial" })).toBeNull();
    expect(screen.queryByText("No contract on record")).toBeNull();
  });

  it("draws no figure for an agreement nobody priced", async () => {
    stubFetch(companyBackstop, {
      // No `base_currency`, so neither sum can be stated in any currency.
      // formatMoneyOrAbsent exists because half a money pair must never
      // render as euros; here there is no pair at all, and the reading omits
      // the figure instead of inventing its units.
      company360: viewWith({ active_count: 1, cancellation_pending: false }),
    });

    render(<CompanyScreen id="o-1" />);
    await openDealsTab();

    const block = await waitFor(() =>
      contractBlock("Under contract · 1 active"),
    );
    expect(block.textContent).not.toMatch(/[€$]|\/ year/);
  });

  it("says nothing about renewal when no agreement names a date", async () => {
    stubFetch(companyBackstop, {
      company360: viewWith({
        active_count: 1,
        cancellation_pending: false,
        base_currency: "EUR",
        annualized_value_minor_base: 12_000_000,
      }),
    });

    render(<CompanyScreen id="o-1" />);
    await openDealsTab();

    const block = await waitFor(() =>
      contractBlock("Under contract · 1 active"),
    );
    expect(block.textContent).toContain("120,000");
    // An unrecorded renewal is an absence. A card that filled it in from the
    // start date plus a year would be inventing the one date a reader plans
    // around.
    expect(block.textContent).not.toContain("Renews");
  });

  it("draws no contract block at all for a reader with no contract grant", async () => {
    // Absent is a fact about the READER, not about the account: this reader
    // cannot be told there are no agreements, because nobody counted.
    stubFetch(companyBackstop, { company360: viewWith(undefined) });

    render(<CompanyScreen id="o-1" />);
    await openDealsTab();

    await screen.findByRole("heading", { name: "Deals" });
    expect(screen.queryByRole("heading", { name: "Commercial" })).toBeNull();
    expect(screen.queryByText(/Under contract/)).toBeNull();
    expect(screen.queryByText("No contract on record")).toBeNull();
  });
});
