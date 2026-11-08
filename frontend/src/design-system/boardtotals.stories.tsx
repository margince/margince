// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { ColumnTotals } from "./boardtotals";
import type { BoardMoneyColumn } from "./pipelineboard";

// The money that heads one stage of the deals board, drawn at a column's width.
const meta: Meta<typeof ColumnTotals> = {
  title: "Components/Text and data display/Board column totals",
  component: ColumnTotals,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div className="board-col-sub" style={{ width: 272 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ColumnTotals>;

function stage(extra: Partial<BoardMoneyColumn>): BoardMoneyColumn {
  return {
    stage: "proposal",
    label: "Proposal",
    deals: [],
    probabilityPct: 10,
    rawMinor: 106_254_800,
    weightedMinor: 10_625_480,
    currency: "EUR",
    ...extra,
  };
}

// The largest stage on the board: its bar runs the full track.
export const LargestStage: Story = {
  args: { money: stage({}), scaleMinor: 106_254_800 },
};

// A quarter of the largest stage, with deals left out and converted money.
export const WithNotes: Story = {
  args: {
    money: stage({
      rawMinor: 26_500_000,
      weightedMinor: 7_969_110,
      probabilityPct: 30,
      unpricedCount: 6,
      asOf: "2026-11-03T08:00:00Z",
    }),
    scaleMinor: 106_254_800,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("6 not priced")).toBeInTheDocument();
    await userEvent.tab();
    await expect(canvas.getByText("€265k")).toHaveFocus();
    const tip = await within(document.body).findByRole("tooltip");
    await expect(tip).toHaveTextContent(/6 deals have no price/);
  },
};

// No shared scale, as on a board whose totals name two currencies: no bar.
export const WithoutScale: Story = {
  args: { money: stage({}), scaleMinor: 0 },
};

// A stage with no measured total shows the absent mark and no bar.
export const NoTotal: Story = {
  args: {
    money: stage({ rawMinor: null, weightedMinor: null }),
    scaleMinor: 106_254_800,
  },
};
