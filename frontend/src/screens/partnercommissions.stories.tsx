// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PartnerCommissions } from "./partnercommissions";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What a partner has earned. The strip on top is the one figure the panel is
// opened for — what is still OWED, one card per currency because two currencies
// added together is a number that means nothing — and it is drawn only while
// there is something to owe: a settled ledger shows its rows and no strip, and
// an empty ledger shows neither.

const meta: Meta = {
  title: "Records/Partner/Commission",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type CommissionEntry = components["schemas"]["CommissionEntry"];

function entry(over: Partial<CommissionEntry>): CommissionEntry {
  return {
    id: "c-1",
    deal_id: "d-1",
    partner_company_id: "o-1",
    status: "accrued",
    attribution_at_accrual: "sourced",
    margin_tier_at_accrual: "tier2_20",
    rate_bps: 2000,
    basis_amount_minor: 4_500_000,
    currency: "EUR",
    amount_minor: 900_000,
    captured_by: "human:x",
    version: 1,
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    ...over,
  } as unknown as CommissionEntry;
}

function Panel({ entries }: Readonly<{ entries: CommissionEntry[] }>) {
  installFetchStub({
    "GET /me": meRoute({ commission: ["read", "update"] }),
    "GET /commissions": () =>
      jsonResponse({
        data: entries,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /deals/d-1": () => jsonResponse({ id: "d-1", name: "Depot rollout" }),
    "GET /deals/d-2": () =>
      jsonResponse({ id: "d-2", name: "Spare parts portal" }),
  });
  return (
    <StoryProviders>
      <PartnerCommissions companyId="o-1" />
    </StoryProviders>
  );
}

// Money still owed in two currencies: two cards, never one sum. The accrued
// row offers its decisions; the paid one is settled and offers nothing.
export const OwedInTwoCurrencies: Story = {
  render: () => (
    <Panel
      entries={[
        entry({}),
        entry({
          id: "c-2",
          deal_id: "d-2",
          status: "approved",
          currency: "CHF",
          basis_amount_minor: 1_250_000,
          amount_minor: 250_000,
        }),
        entry({
          id: "c-3",
          status: "paid",
          amount_minor: 300_000,
          basis_amount_minor: 1_500_000,
        }),
      ]}
    />
  ),
};

// Everything paid or reversed: the ledger stays, the strip does not. A card
// reading "0" would spend a slot saying there is nothing to say.
export const AllSettled: Story = {
  render: () => (
    <Panel
      entries={[
        entry({ status: "paid" }),
        entry({ id: "c-2", deal_id: "d-2", status: "void" }),
      ]}
    />
  ),
};

export const NothingEarned: Story = { render: () => <Panel entries={[]} /> };
