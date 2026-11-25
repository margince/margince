// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { type ListColumn, ListTable } from "./listtable";

const meta: Meta = {
  title: "Components/Text and data display/List grid phone rows",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

type Deal = { id: string; name: string; amount: string; stage: string };

const deals: Deal[] = [
  { id: "d1", name: "Fleet retrofit", amount: "€48,000", stage: "Proposal" },
  { id: "d2", name: "Harbour lighting", amount: "€12,500", stage: "Qualified" },
];

const columns: ListColumn<Deal>[] = [
  { key: "name", header: "Deal", fixed: true, cell: (deal) => deal.name },
  {
    key: "amount",
    header: "Amount",
    numeric: true,
    phone: "end",
    cell: (deal) => deal.amount,
  },
  {
    key: "stage",
    header: "Stage",
    phone: "caption",
    cell: (deal) => deal.stage,
  },
];

/**
 * On a phone each row is a card. The name leads, the amount ends its line,
 * and the stage sits in the caption under them.
 */
export const PhoneRows: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <LocaleProvider initial="en">
      <ListTable
        rows={deals}
        columns={columns}
        rowKey={(deal) => deal.id}
        unit="deals"
      />
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Fleet retrofit");
    const caption = canvasElement.querySelector('[data-phone="caption"]');
    await expect(caption?.textContent).toContain("Proposal");
    const end = canvasElement.querySelector('[data-phone="end"]');
    await expect(end?.textContent).toBe("€48,000");
  },
};
