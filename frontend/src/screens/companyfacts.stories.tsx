// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyFacts } from "./companyfacts";
import { StoryProviders } from "./story-utils";

// The account's standing, in RecordView's `controls` slot: what the open
// pipeline is worth, how much work is in flight, whose account it is.
//
// The stories are the states that are one pixel apart on screen and opposite
// facts about the account. "No open deals" says the account has none;
// "hidden" says the reader may not see them. A box that drew those alike
// would tell a rep with no deal grant that a live account is empty.

type View = components["schemas"]["Organization360"];
type Organization = components["schemas"]["Organization"];

const page = { has_more: false, next_cursor: null };

const org = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as unknown as Organization;

const base = {
  as_of: "2026-08-25T09:00:00Z",
  organization: org,
  sections_omitted: [],
  deals: {
    data: [],
    page,
    won_lifetime: { amount_minor: 0, currency: "EUR" },
    lost_count: 0,
  },
  projects: [],
  projects_page: page,
  state_strip: {
    account: { lifecycle: "customer", relationship_types: [] },
    commercial: {
      open_count: 0,
      stalled_count: 0,
      priced_count: 0,
      converted_count: 0,
    },
  },
} as unknown as View;

function Box({ view }: Readonly<{ view?: View }>) {
  return (
    <StoryProviders>
      {/* Narrow on purpose: the box lives in a record head's right-hand slot,
          and a fact that wraps there is a fact nobody reads. */}
      <div style={{ maxWidth: 360 }}>
        <CompanyFacts org={org} view={view} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Box> = {
  title: "Screens/Company/Facts",
  component: Box,
};
export default meta;
type Story = StoryObj<typeof Box>;

// A working account: money on the table, work under way, an owner.
export const Populated: Story = {
  render: () => (
    <Box
      view={
        {
          ...base,
          deals: {
            data: [{ deal_id: "d-1" }, { deal_id: "d-2" }],
            page,
            won_lifetime: { amount_minor: 18_000_000, currency: "EUR" },
            lost_count: 1,
          },
          projects: [
            { project_id: "p-1", name: "Rollout", phase: "delivering" },
          ],
          state_strip: {
            account: { lifecycle: "customer", relationship_types: [] },
            commercial: {
              open_count: 2,
              stalled_count: 0,
              priced_count: 2,
              converted_count: 0,
              open_pipeline_minor_base: 6_400_000,
              base_currency: "EUR",
            },
          },
        } as unknown as View
      }
    />
  ),
};

// Each half pluralises on its own count, so one project reads "1 project"
// beside two deals. One shared template printed "1 projects" here.
export const OneOfEach: Story = {
  render: () => (
    <Box
      view={
        {
          ...base,
          deals: {
            data: [{ deal_id: "d-1" }],
            page,
            won_lifetime: { amount_minor: 0, currency: "EUR" },
            lost_count: 0,
          },
          projects: [
            { project_id: "p-1", name: "Rollout", phase: "delivering" },
          ],
        } as unknown as View
      }
    />
  ),
};

// Deals nobody has costed. A dash here would read as "we do not know what
// they are worth", which is the one reading it is not.
export const UnpricedPipeline: Story = {
  render: () => (
    <Box
      view={
        {
          ...base,
          deals: {
            data: [{ deal_id: "d-1" }, { deal_id: "d-2" }, { deal_id: "d-3" }],
            page,
            won_lifetime: { amount_minor: 0, currency: "EUR" },
            lost_count: 0,
          },
          state_strip: {
            account: { lifecycle: "opportunity", relationship_types: [] },
            commercial: {
              open_count: 3,
              stalled_count: 0,
              priced_count: 0,
              converted_count: 0,
            },
          },
        } as unknown as View
      }
    />
  ),
};

// Readable and genuinely empty. A FACT about the account, and the state the
// one below must not be confused with.
export const NothingOpen: Story = { render: () => <Box view={base} /> };

// The deal grant is missing. `state_strip` is still present — only its
// `commercial` half is gone — so the absence is about the READER.
export const PipelineWithheld: Story = {
  render: () => (
    <Box
      view={
        {
          ...base,
          state_strip: {
            account: { lifecycle: "customer", relationship_types: [] },
          },
        } as unknown as View
      }
    />
  ),
};

// One half unreadable means NO count at all: "1 project" on an account whose
// deals the reader cannot see reads as an account with no deals.
export const InFlightWithheld: Story = {
  render: () => (
    <Box
      view={
        {
          ...base,
          projects: undefined,
          sections_omitted: ["projects"],
        } as unknown as View
      }
    />
  ),
};

// The composite has not answered yet. A third state again: neither withheld
// nor empty, and drawing either would state a fact the page does not have.
export const Reading: Story = { render: () => <Box view={undefined} /> };
