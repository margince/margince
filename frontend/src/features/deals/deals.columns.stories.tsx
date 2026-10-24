// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { PipelineBoard } from "../../design-system/pipelineboard";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { buildStageTotals } from "../../lib/deals.queries";
import type { Deal, Stage } from "./deals";
import { buildColumns } from "./deals.columns";

const meta: Meta = {
  title: "Records/Deals/Converted board",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const stage: Stage = {
  id: "qualified",
  pipeline_id: "sales",
  name: "Qualified",
  position: 0,
  semantic: "open",
  win_probability: 50,
};

const deals: Deal[] = [
  {
    id: "priced",
    name: "Fleet retrofit",
    currency: "USD",
    amount_minor: 100_000,
  },
  {
    id: "unpriced",
    name: "Solar pilot",
    currency: "JPY",
    amount_minor: 200_000,
  },
].map<Deal>((deal) => ({
  ...deal,
  pipeline_id: stage.pipeline_id,
  stage_id: stage.id,
  status: "open",
  source: "manual",
  captured_by: "human:demo",
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
}));

export const PartialCoverage: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <PipelineBoard
        columns={buildColumns(
          [stage],
          deals,
          buildStageTotals(
            [
              {
                stage_id: stage.id,
                deals: 2,
                base_minor: 90_000,
                weighted_base_minor: 45_000,
                priced_deals: 1,
              },
            ],
            "EUR",
            "2026-10-01T23:30:00Z",
            UTC_ZONE,
          ),
          { marks: new Map(), unreadable: new Set() },
        )}
        cardHref={(deal) => `#/deals/${deal.id}`}
        zone={UTC_ZONE}
      />
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Fleet retrofit")).toBeVisible();
    await expect(canvas.getByText("Solar pilot")).toBeVisible();
    await expect(
      canvas.getByText("1 deal not priced · excluded"),
    ).toBeVisible();
    await expect(canvas.getByText(/01\/10\/2026/)).toBeVisible();
    await expect(canvas.getByText(/€900/)).toBeVisible();
  },
};
