// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyWorkCard } from "./companywork";
import { StoryProviders } from "./story-utils";

// What is moving on the account, and for each piece the one reason it wants a
// person today.
//
// Deals only — the account's projects have their one home in the ProjectLinks
// section. Two rules run through every story here and neither is visible in a
// single screenshot: every line is one record's own story, and a section a
// reader may not see is never drawn as a section with nothing in it — "no
// open deals" on an account with a full pipeline is the worst thing this card
// can say.

type View = components["schemas"]["Organization360"];

const page = { has_more: false, next_cursor: null };

const base = {
  as_of: "2026-08-25T09:00:00Z",
  organization: {
    id: "o-1",
    display_name: "Brandt Automotive GmbH",
    captured_by: "human:u1",
    source: "manual",
    version: 1,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  sections_omitted: [],
  deals: {
    data: [],
    page,
    won_lifetime: { amount_minor: 0, currency: "EUR" },
    lost_count: 0,
  },
  projects: [],
  projects_page: page,
} as unknown as View;

// Shaped as Organization360Deal serves it: `status` is required, and the
// money is a nested Money rather than two loose fields. The loose spelling
// typechecked through the cast below and silently drew no figure at all —
// the card reads `deal.amount.amount_minor`.
const deal = {
  deal_id: "d-1",
  name: "Shopsystem-Migration — zweiter Mandant",
  status: "open",
  stage_name: "Qualified",
  amount: { amount_minor: 6_400_000, currency: "EUR" },
  expected_close_date: "2026-09-28",
  stalled: false,
};

function Card({ view, loading }: Readonly<{ view?: View; loading?: boolean }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <CompanyWorkCard view={view} loading={loading} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Card> = {
  title: "Records/Company 360/Work in flight",
  component: Card,
};
export default meta;
type Story = StoryObj<typeof Card>;

// Rows and nothing wrong with any of them: the baseline a reader learns the
// card's shape from.
export const Populated: Story = {
  render: () => (
    <Card
      view={
        {
          ...base,
          deals: { ...base.deals, data: [deal] },
        } as unknown as View
      }
    />
  ),
};

// One attention fact per row, which is the card's whole point: a task nobody
// did.
export const WithAttention: Story = {
  render: () => (
    <Card
      view={
        {
          ...base,
          deals: {
            ...base.deals,
            data: [
              {
                ...deal,
                attention: {
                  kind: "overdue_task",
                  title: "Angebot nachfassen",
                  who: "Sofia Meier",
                  due_at: "2026-08-14T09:00:00Z",
                },
              },
            ],
          },
        } as unknown as View
      }
    />
  ),
};

// Readable and genuinely empty. A FACT about the account — and the state the
// overview reads to decide the growth-fit card takes this slot.
export const NothingInFlight: Story = { render: () => <Card view={base} /> };

// The rows are readable but the activity grant is not, so no attention fact
// could be derived for any of them. The rows render and the card SAYS the
// statuses are incomplete — without that line a piece of work with a hidden
// overdue task reads exactly like one with nothing outstanding.
export const StatusesWithheld: Story = {
  render: () => (
    <Card
      view={
        {
          ...base,
          deals: { ...base.deals, data: [deal] },
          // The refusal is recorded BOTH ways: the assembler names the section
          // it could not read before it sets the flag, so a story carrying the
          // flag alone is a payload the endpoint cannot emit.
          sections_omitted: ["activities"],
          attention_withheld: true,
        } as unknown as View
      }
    />
  ),
};

// The section was cut short by its cap, so the count is a floor. "2 in
// flight" on a portfolio account is a number that account does not have.
export const MoreThanFits: Story = {
  render: () => (
    <Card
      view={
        {
          ...base,
          deals: {
            ...base.deals,
            data: [deal],
            // `truncate` sets has_more and nothing else — a nested 360
            // summary is not a paging surface, so there is no cursor to
            // continue from.
            page: { has_more: true, next_cursor: null },
          },
        } as unknown as View
      }
    />
  ),
};

// The composite is still running. `loading` is what separates this from the
// story below: both hand the card no view, and without the flag a read still
// on its way is drawn as one that failed.
export const Loading: Story = { render: () => <Card loading /> };

// The composite FAILED, or answered something this client cannot read. No
// view and no flag — "some of this could not be loaded", which is a different
// sentence from "not yet".
export const Unavailable: Story = { render: () => <Card view={undefined} /> };
