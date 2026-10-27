// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Locale } from "../../i18n";
import { StoryProviders } from "../../testing/story-utils";
import type { Offer } from "./deals";
import { OffersPanel } from "./deals.overview";

// The deal's offers card. Each row's badge names the offer's status in the
// reader's words; the wire's code never reaches the page.
const meta = {
  title: "Records/Deal 360/Offers card",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const draftOffer: Offer = {
  id: "o1",
  deal_id: "d1",
  offer_number: "OFF-0001",
  revision: 1,
  status: "draft",
  currency: "EUR",
  net_minor: 100_000,
  tax_minor: 19_000,
  gross_minor: 119_000,
  ai_generated: false,
  line_items: [],
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

function Card({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <StoryProviders locale={locale}>
      <OffersPanel
        offers={[draftOffer]}
        creating={false}
        locale={locale}
        dealCurrency="EUR"
        onCreate={() => undefined}
      />
    </StoryProviders>
  );
}

async function expectStatusWord(
  canvasElement: HTMLElement,
  title: string,
  word: string,
) {
  const open = await within(canvasElement).findByRole("button", {
    name: title,
  });
  const item = open.closest("li");
  if (item === null) throw new Error("the offer sits outside its list item");
  await expect(within(item).getByText(word)).toBeVisible();
}

export const DraftOffer: Story = {
  name: "Draft offer, status in words",
  render: () => <Card locale="en" />,
  play: async ({ canvasElement }) => {
    await expectStatusWord(canvasElement, "OFF-0001 · version 1", "Draft");
  },
};

export const DraftOfferGerman: Story = {
  name: "Draft offer, German",
  render: () => <Card locale="de" />,
  play: async ({ canvasElement }) => {
    await expectStatusWord(canvasElement, "OFF-0001 · Version 1", "Entwurf");
  },
};
