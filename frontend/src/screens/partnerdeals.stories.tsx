// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PartnerDeals } from "./partnerdeals";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// What a partner brought us. Every row is "this partner brought this deal for
// someone", so the customer column is the one that carries the panel — which is
// why the withheld state below is a story of its own rather than a footnote: a
// customer the reader may not see and a deal nobody linked used to render the
// same em dash, and the panel then read as sloppy data rather than as a limit on
// what this reader is allowed to know.

const meta: Meta = {
  title: "Records/Partner/Sourced deals",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Deal = components["schemas"]["Deal"];

function deal(over: Partial<Deal>): Deal {
  return {
    id: "d-1",
    name: "Depot rollout",
    organization_id: "o-9",
    status: "open",
    currency: "EUR",
    amount_minor: 4_500_000,
    partner_attribution: "sourced",
    version: 1,
    ...over,
  } as unknown as Deal;
}

function Panel({ deals }: Readonly<{ deals: Deal[] }>) {
  installFetchStub({
    "GET /deals": () =>
      jsonResponse({
        data: deals,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /organizations/o-9": () =>
      jsonResponse({ id: "o-9", display_name: "Nordwerk GmbH" }),
  });
  return (
    <StoryProviders>
      <PartnerDeals organizationId="p-1" />
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

// The row this panel exists to get right: `organization_id` is null and the
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
          organization_id: null,
          masked_fields: ["organization_id"],
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
