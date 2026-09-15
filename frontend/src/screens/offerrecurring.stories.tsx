// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { OfferTotalsPanel } from "./offerrecurring";
import { StoryProviders } from "./story-utils";

// An offer's money totals: net, tax and gross, and — only where the offer has a
// recurring part — its annual and committed values. Every figure sits in
// tabular digits so the column lines up against the line list beside it.

type Offer = components["schemas"]["Offer"];

const oneOff: Offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "ANG-2026-0007",
  revision: 1,
  status: "draft",
  currency: "EUR",
  net_minor: 1_000_00,
  tax_minor: 190_00,
  gross_minor: 1_190_00,
  arr_minor: 0,
  ai_generated: false,
  line_items: [],
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// A quarterly line at 3,000 committed for eight quarters: 12,000 a year and
// 24,000 of committed value, shown side by side so neither is read as the other.
const recurring: Offer = {
  ...oneOff,
  net_minor: 24_000_00,
  tax_minor: 4_560_00,
  gross_minor: 28_560_00,
  arr_minor: 12_000_00,
  net_tcv_minor: 24_000_00,
};

const meta: Meta<typeof OfferTotalsPanel> = {
  title: "Records/Offers/Totals",
  component: OfferTotalsPanel,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof OfferTotalsPanel>;

/** Purely one-off work: net, tax and gross, and no recurring rows. */
export const OneOff: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 360 }}>
        <OfferTotalsPanel offer={oneOff} />
      </div>
    </StoryProviders>
  ),
};

/** A recurring component adds the annual and committed figures under gross. */
export const WithRecurringPart: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 360 }}>
        <OfferTotalsPanel offer={recurring} />
      </div>
    </StoryProviders>
  ),
};
