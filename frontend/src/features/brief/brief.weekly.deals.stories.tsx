// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { narratedWeek, type WeeklyReview } from "../../testing/brief.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import {
  ClosedDeals,
  ClosedTotal,
  MovedCount,
  MovedDeals,
} from "./brief.weekly.deals";
import "./brief.weekly.css";

// The weekly review's two deal lists, each under the label its total sits at.
const meta: Meta = {
  title: "Shell/Home/Weekly deals",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

// The won deal closed at a stated amount; the lost one names none, so its
// amount reads as a dash.
const week: WeeklyReview = {
  ...narratedWeek,
  deals: narratedWeek.deals.map((deal) =>
    deal.outcome === "won"
      ? { ...deal, amount_minor_at_close: 96_500_00, currency_at_close: "EUR" }
      : deal,
  ),
};
const closed = week.deals.filter((deal) => deal.outcome !== "moved");
const moved = week.deals.filter((deal) => deal.outcome === "moved");

function Lists() {
  return (
    <StoryProviders>
      <Panel title="Closed deals" titleAction={<ClosedTotal review={week} />}>
        <ClosedDeals review={week} deals={closed} />
      </Panel>
      <Panel
        title="Pipeline movement"
        titleAction={<MovedCount count={week.counts.deals_moved} />}
      >
        <MovedDeals deals={moved} />
      </Panel>
    </StoryProviders>
  );
}

/** Won and lost with their outcome and amount; a moved deal with its stage. */
export const DealLists: Story = {
  render: () => <Lists />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("link", { name: "Weber Rahmenvertrag" }),
    ).toBeVisible();
    await expect(canvas.getByText("won")).toBeVisible();
    await expect(canvas.getByText("lost")).toBeVisible();
    await expect(canvas.getByText("€96,500.00")).toBeVisible();
    await expect(canvas.getByText("—")).toBeVisible();
    await expect(canvas.getByText("Proposal sent")).toBeVisible();
    await expect(canvas.getByText(/Won .* · Lost /)).toBeVisible();
  },
};

export const DealListsDark: Story = {
  ...DealLists,
  globals: { theme: "dark" },
};

export const DealListsPhone: Story = { ...DealLists, tags: ["uat-phone"] };
