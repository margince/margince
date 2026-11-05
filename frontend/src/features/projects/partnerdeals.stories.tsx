// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { Locale } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { PartnerDeals } from "./partnerdeals";

// What a partner brought us. Every row is "this partner brought this deal for
// someone", so the customer column is the one that carries the panel — which is
// why the withheld state below is a story of its own rather than a footnote: a
// customer the reader may not see and a deal nobody linked used to render the
// same em dash, and the panel then read as sloppy data rather than as a limit on
// what this reader is allowed to know.

const meta: Meta = {
  title: "Records/Company 360/Partner tab/Sourced deals",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Deal = components["schemas"]["Deal"];

function deal(over: Partial<Deal>): Deal {
  return {
    id: "d-1",
    name: "Depot rollout",
    company_id: "o-9",
    pipeline_id: null,
    stage_id: null,
    status: "open",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    currency: "EUR",
    amount_minor: 4_500_000,
    partner_attribution: "sourced",
    version: 1,
    ...over,
  };
}

function Panel({
  deals,
  locale = "en",
}: Readonly<{ deals: Deal[]; locale?: Locale }>) {
  installFetchStub({
    "GET /deals": () =>
      jsonResponse({
        data: deals,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /companies/o-9": () =>
      jsonResponse({ id: "o-9", display_name: "Nordwerk GmbH" }),
  });
  return (
    <StoryProviders locale={locale}>
      <PartnerDeals companyId="p-1" />
    </StoryProviders>
  );
}

export const SourcedAndInfluenced: Story = {
  render: () => (
    <Panel
      deals={[
        deal({}),
        deal({
          id: "d-2",
          name: "Second site — spare parts portal",
          partner_attribution: "influenced",
          amount_minor: 1_250_000,
        }),
      ]}
    />
  ),
};

// The row this panel exists to get right: `company_id` is null and the
// field is named in `masked_fields`, so the customer is WITHHELD rather than
// unset, and the column says so instead of drawing the em dash an unlinked deal
// gets.
export const CustomerWithheld: Story = {
  render: () => (
    <Panel
      deals={[
        deal({
          id: "d-3",
          name: "Regional expansion",
          company_id: null,
          masked_fields: ["company_id"],
        }),
        deal({}),
      ]}
    />
  ),
};

// A masked amount is null and the money column stays an em dash on purpose: the
// row still names the deal and its customer, which is what a partner page is
// for.
export const AmountWithheld: Story = {
  render: () => (
    <Panel
      deals={[
        deal({
          id: "d-4",
          amount_minor: null,
          currency: null,
          masked_fields: ["amount_minor"],
        }),
      ]}
    />
  ),
};

export const NothingBrought: Story = { render: () => <Panel deals={[]} /> };

/** Each deal's status in the reader's words, never the stored value. */
export const Vietnamese: Story = {
  render: () => (
    <Panel
      locale="vi"
      deals={[
        deal({}),
        deal({ id: "d-5", name: "Spare parts", status: "won" }),
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Đang mở")).toBeVisible();
    await expect(canvas.getByText("Thắng")).toBeVisible();
  },
};
