// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DealsSection } from "./companyraildeals";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The rail's open-pipeline section on its own: the ranked top deals, each with
// its amount in tabular figures so the column of money lines up down the rail,
// and the note that says why a deal needs a move ahead of its stage and close.
// The empty frame is the account between cycles, which carries the way to the
// Deals tab rather than the create verb.

const meta: Meta = {
  title: "Records/Company rail/Deals",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type View = components["schemas"]["Company360"];
type Deal = components["schemas"]["Company360Deal"];

const page = { has_more: false, next_cursor: null };

const company: components["schemas"]["Company"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const openDeals: Deal[] = [
  {
    deal_id: "d-1",
    name: "Fleet electrification pilot",
    status: "open",
    stage_name: "Proposal",
    amount: { amount_minor: 184_000_00, currency: "EUR" },
    expected_close_date: "2026-10-15",
    stalled: false,
  },
  {
    deal_id: "d-2",
    name: "Charging depot retrofit",
    status: "open",
    stage_name: "Negotiation",
    amount: { amount_minor: 42_500_00, currency: "EUR" },
    expected_close_date: "2026-09-30",
    stalled: false,
    attention: { kind: "overdue_task", title: "Send revised quote" },
  },
  {
    deal_id: "d-3",
    name: "Service contract renewal",
    status: "open",
    stage_name: "Qualify",
    amount: { amount_minor: 9_800_00, currency: "EUR" },
    stalled: true,
  },
];

function view(deals: Deal[], lostCount: number): View {
  return {
    as_of: "2026-09-01T09:00:00Z",
    company,
    sections_omitted: [],
    deals: {
      data: deals,
      page,
      won_lifetime: { amount_minor: 0, currency: "EUR" },
      lost_count: lostCount,
    },
  };
}

function Section({ data }: Readonly<{ data: View }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"], deal: ["create"] }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-1", display_name: "Mira Voss" }],
        page,
      }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <DealsSection view={data} loading={false} onTab={() => {}} />
      </div>
    </StoryProviders>
  );
}

// The deal with an overdue task and the stalled one rank ahead of the larger
// quiet deal; each row's amount sits in tabular figures.
export const OpenPipeline: Story = {
  render: () => <Section data={view(openDeals, 0)} />,
};

// No open deals, but two lost ones: the account is between cycles, so the
// empty state says so and offers the Deals tab rather than a new deal.
export const BetweenCycles: Story = {
  render: () => <Section data={view([], 2)} />,
};
