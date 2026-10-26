// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactDealsTab } from "./contactdeals";
import { populated, type View } from "./contactpage.fixtures";

// The contact's deals as one list. Each row has its money, stage, close date
// and company over the seat. The open deal's committee follows once under the
// list. The frames: two seats with the committee named (one seat with no role),
// and a contact on no deal.

const meta: Meta = {
  title: "Records/Contact 360/Deals tab",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const twoSeats: View = {
  ...populated,
  deal_roles: {
    data: [
      {
        relationship_id: "r-1",
        deal_id: "d-1",
        deal_title: "Fleet retrofit, 40 vehicles",
        deal_stage: "Proposal",
        role: "champion",
      },
      {
        relationship_id: "r-2",
        deal_id: "d-2",
        deal_title: "Phase two services",
        deal_stage: "Qualification",
        role: "user",
      },
    ],
    page: { has_more: false },
  },
  commercial: {
    deal: { deal_id: "d-1", title: "Fleet retrofit, 40 vehicles" },
    role: "champion",
    committee: [
      { contact_id: "c-1", full_name: "Dana Buyer", role: "champion" },
      { contact_id: "c-2", full_name: "Markus Ott", role: "" },
    ],
  },
};

const deal = (id: string, amount: number, close: string) =>
  jsonResponse({
    id,
    title: id,
    amount_minor: amount,
    currency: "EUR",
    expected_close_date: close,
    company_id: "co-1",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  });

function Deals({ view }: Readonly<{ view: View }>) {
  stubWithSession(
    {
      "GET /deals/d-1": () => deal("d-1", 4_800_000, "2026-09-30"),
      "GET /deals/d-2": () => deal("d-2", 1_200_000, "2026-10-31"),
      "GET /companies/co-1": () =>
        jsonResponse({
          id: "co-1",
          display_name: "Brandt Automotive GmbH",
          source: "manual",
          captured_by: "human:u-1",
          created_at: "2026-06-01T08:00:00Z",
          updated_at: "2026-06-01T08:00:00Z",
        }),
    },
    { contact: ["read"], relationship: ["create"] },
  );
  return (
    <StoryProviders>
      <ContactDealsTab view={view} />
    </StoryProviders>
  );
}

export const Seats: Story = { render: () => <Deals view={twoSeats} /> };

export const SeatsDark: Story = {
  ...Seats,
  globals: { theme: "dark" },
};

export const SeatsPhone: Story = { ...Seats, tags: ["uat-phone"] };

export const NoDeals: Story = {
  render: () => (
    <Deals
      view={{
        ...populated,
        commercial: undefined,
        deal_roles: { data: [], page: { has_more: false } },
      }}
    />
  ),
};
