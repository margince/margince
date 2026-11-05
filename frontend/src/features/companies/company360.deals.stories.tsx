// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import type { Company360 } from "./company360";
import { DealsCard } from "./company360.deals";

// The account's deals card. Each row's status is the one deal status badge,
// so a status this build has no word for still reads as sent.
const meta = {
  title: "Records/Company 360/Deals card",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;
type Company360Deal = components["schemas"]["Company360Deal"];

const deals: Company360Deal[] = [
  {
    deal_id: "d-1",
    name: "Fleet retrofit",
    status: "open",
    stage_name: "Solution Discovery",
    amount: { amount_minor: 4_800_000, currency: "EUR" },
    expected_close_date: "2026-09-30",
    stalled: false,
  },
  {
    deal_id: "d-2",
    name: "Phase one licence",
    status: "won",
    stalled: false,
  },
];

const view: Company360 = {
  as_of: "2026-08-13T09:00:00Z",
  company: {
    id: "o-1",
    display_name: "Brandt Automotive GmbH",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  },
  sections_omitted: [],
  deals: {
    data: deals,
    page: { has_more: false, next_cursor: null },
    won_lifetime: { amount_minor: 450_000, currency: "EUR" },
    lost_count: 1,
  },
};

// A newer server's status, read the way the client reads any body: parsed
// JSON typed by the contract and never checked against its enum.
const skewedView: Company360 = JSON.parse(
  JSON.stringify({
    ...view,
    deals: {
      ...view.deals,
      data: [{ ...deals[0], status: "on_hold" }],
    },
  }),
);

// The deal's row, found by the name it reads with, so a status word is read
// from that row alone.
async function rowOf(canvasElement: HTMLElement, deal: string) {
  return within(
    await within(canvasElement).findByRole("row", { name: new RegExp(deal) }),
  );
}

export const OpenAndWon: Story = {
  name: "Open and won deals",
  render: () => (
    <StoryProviders>
      <DealsCard view={view} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const open = await rowOf(canvasElement, "Fleet retrofit");
    await expect(open.getByText("Open")).toBeVisible();
    await expect(open.queryByText("Won")).toBeNull();
    const won = await rowOf(canvasElement, "Phase one licence");
    await expect(won.getByText("Won")).toBeVisible();
  },
};

export const UnknownStatus: Story = {
  name: "Status this build has no word for",
  render: () => (
    <StoryProviders>
      <DealsCard view={skewedView} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const row = await rowOf(canvasElement, "Fleet retrofit");
    await expect(row.getByText("on_hold")).toBeVisible();
  },
};
