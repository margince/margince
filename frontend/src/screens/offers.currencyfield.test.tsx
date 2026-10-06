// @vitest-environment happy-dom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { OfferCurrencyField } from "./offers.currencyfield";
import { StoryProviders } from "./story-utils";

afterEach(cleanup);

type Offer = components["schemas"]["Offer"];
type Line = NonNullable<Offer["line_items"]>[number];

// A whole line, because the component takes the offer's own list and a partial
// row would let the test disagree with the contract about what a line is. Only
// the price varies: it is the one field this control reads.
const BASE: Line = {
  id: "00000000-0000-7000-8000-000000000001",
  position: 1,
  description: "Implementation",
  unit: "unit",
  quantity: 1,
  unit_price_minor: 0,
  discount_pct: 0,
  tax_rate: 19,
  line_net_minor: 0,
  line_tax_minor: 0,
  line_total_minor: 0,
  // Grounded: this fixture states its price on purpose. An UNGROUNDED line is
  // the other way a 0 gets here, and the constraint behind it requires exactly
  // that 0 — which is why the control reads the figure and not this flag.
  price_grounded: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const priced = (unit_price_minor: number): Line => ({
  ...BASE,
  unit_price_minor,
});

function field(lineItems: Line[]) {
  return render(
    <StoryProviders>
      <OfferCurrencyField
        value="EUR"
        lineItems={lineItems}
        onChange={() => {}}
      />
    </StoryProviders>,
  );
}

describe("OfferCurrencyField", () => {
  it("offers the currency while no line states a price", () => {
    field([]);
    expect(screen.getByRole("combobox")).toBeEnabled();
    expect(
      screen.queryByText(en["offer.currencyFixedByLines"]),
    ).not.toBeInTheDocument();
  });

  it("stops offering it once a line states one, and says why", () => {
    field([priced(9500)]);
    expect(screen.getByRole("combobox")).toBeDisabled();
    // The rule, not a refusal of this attempt: the reader learns it before
    // trying rather than by being told no.
    expect(
      screen.getByText(en["offer.currencyFixedByLines"]),
    ).toBeInTheDocument();
  });

  // Zero is the same amount in every currency, so a line stating it is not a
  // figure the change could re-mean. The server counts these lines the same way
  // (`refuseRepricingByCurrency`), and a control that refused here would refuse
  // a change the server would take.
  it("still offers it when the only line costs nothing", () => {
    field([priced(0)]);
    expect(screen.getByRole("combobox")).toBeEnabled();
  });

  // A line has no way to say "no price" but to state 0 — `unit_price_minor` is
  // NOT NULL — so a draft of zeros is the unpriced draft, and one line stating a
  // figure among them is enough to fix the currency.
  it("stops offering it when one line of several states a price", () => {
    field([priced(0), priced(0), priced(1)]);
    expect(screen.getByRole("combobox")).toBeDisabled();
  });
});
