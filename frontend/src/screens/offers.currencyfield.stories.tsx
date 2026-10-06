import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { OfferCurrencyField } from "./offers.currencyfield";
import { StoryProviders } from "./story-utils";

type Offer = components["schemas"]["Offer"];
type Line = NonNullable<Offer["line_items"]>[number];

// A whole line, because the component takes the offer's own list. Only the price
// varies between these stories: it is the one field this control reads.
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

const meta: Meta<typeof OfferCurrencyField> = {
  title: "Records/Offers/Currency control",
  component: OfferCurrencyField,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: { value: "EUR", onChange: () => {} },
};
export default meta;
type Story = StoryObj<typeof OfferCurrencyField>;

// A draft with nothing priced yet: the currency is still free to move, because
// there is no figure for it to re-mean.
export const Offered: Story = { args: { lineItems: [] } };

// One priced line is enough to fix it. The hint is the RULE, in the field's hint
// slot rather than its error slot — the reader learns it before trying, instead
// of by being told no.
export const FixedByAPricedLine: Story = {
  args: { lineItems: [priced(9500)] },
};

// Zero is the same amount in every currency, so a draft of zeros is the unpriced
// draft and the control still offers the change. This is also the state an
// ungrounded AI draft leaves behind, which is the case a reviewer caught the
// first version of this control getting wrong.
export const StillOfferedWhenEveryLineCostsNothing: Story = {
  args: { lineItems: [priced(0), priced(0)] },
};

export const OfferedDark: Story = {
  args: { lineItems: [] },
  globals: { theme: "dark" },
};

// The disabled control and its hint in dark: a hint is a derived colour, so it
// can be right in light and wrong here.
export const FixedByAPricedLineDark: Story = {
  args: { lineItems: [priced(9500)] },
  globals: { theme: "dark" },
};
