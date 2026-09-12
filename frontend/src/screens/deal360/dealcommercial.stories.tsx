// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealCommercial } from "./dealcommercial";

// The commercial context in a deal's side pane.
//
// The states worth seeing are the partial ones: a deal with a motion and no
// priority must show one row rather than a placeholder for the other, and a
// deal carrying a RETIRED source must still render that source's label — a
// panel that dropped it would report the channel as unrecorded.

type Deal = components["schemas"]["Deal"];
type AcquisitionSource = components["schemas"]["AcquisitionSource"];

const deal = (over: Partial<Deal>): Deal =>
  ({
    id: "d1",
    name: "Seasonal payroll",
    status: "open",
    source: "manual",
    captured_by: "u1",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...over,
  }) as Deal;

const sources: AcquisitionSource[] = [
  {
    id: "s1",
    key: "referral",
    label: "Referral",
    sort_order: 30,
    active: true,
    system: true,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "s2",
    key: "roadshow",
    label: "Roadshow",
    sort_order: 90,
    active: false,
    system: false,
    version: 2,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-02-01T00:00:00Z",
  },
];

const meta: Meta<typeof DealCommercial> = {
  title: "Records/Deal commercial context",
  component: DealCommercial,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 360 }}>
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DealCommercial>;

export const Full: Story = {
  args: {
    deal: deal({
      commercial_motion: "new_business",
      priority: "high",
      acquisition_source: "referral",
    }),
    sources,
  },
};

// One field answered, two not. The unanswered ones are absent, not blank rows:
// "nobody said" is different from "the answer is empty".
export const Partial: Story = {
  args: { deal: deal({ priority: "medium" }), sources },
};

// A source that has since been retired still renders its label here. The deal
// carries the key, so the page must be able to say what it means.
export const RetiredSource: Story = {
  args: { deal: deal({ acquisition_source: "roadshow" }), sources },
};

// Recurring revenue with an EXACT monthly reading — 12,000.00 a year divides
// into 1,000.00 a month with nothing left over, so no approximation marker.
export const RecurringExact: Story = {
  args: {
    deal: deal({
      commercial_motion: "renewal",
      expected_arr_minor: 1_200_000,
      currency: "EUR",
    }),
    sources,
  },
};

// The same panel where the division does NOT come out even. 100.00 a year is
// 8.33 a month and change, so the monthly row is marked approximate: a reader
// who multiplies it back will not reach the annual figure.
export const RecurringApproximate: Story = {
  args: {
    deal: deal({ expected_arr_minor: 10_000, currency: "EUR" }),
    sources,
  },
};

// A subscription deal with no one-off amount at all. This row was illegal
// before the money pairing was rewritten, and it is the shape the whole slice
// exists for.
export const RecurringOnlyInAZeroDigitCurrency: Story = {
  args: {
    deal: deal({ expected_arr_minor: 1_000_000, currency: "JPY" }),
    sources,
  },
};

// Renders nothing at all when no one has recorded any of them.
export const Unset: Story = { args: { deal: deal({}), sources } };
