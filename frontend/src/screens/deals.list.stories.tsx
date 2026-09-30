// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { DealsScreen } from "./deals";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Records/Deals/Page",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const deal = {
  id: "d1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s1",
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

// The pipeline board as the list surface's BODY, which is the whole point of
// this story: the saved-view rail, the count, the filter bar and the archived
// toggle stand above the board exactly as they stand above the table, because
// both views read one query. Rendered instead of the surface, the board took all
// four off screen and left the reader unable to see — or undo — what had
// narrowed the pipeline.
const boardStages = [
  {
    id: "s1",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "s2",
    pipeline_id: "pl",
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 60,
  },
];

// Relative to the moment the story renders, so the mail chip says "10 d ago"
// on every day the canvas is opened rather than counting up from a fixed date.
function daysAgo(days: number): string {
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

const boardDeals = [
  {
    ...deal,
    id: "b1",
    name: "Fleet retrofit",
    company_id: "o1",
    // The buyer wrote last, two days ago.
    last_email: { occurred_at: daysAgo(2), direction: "inbound" },
  },
  {
    ...deal,
    id: "b2",
    name: "Depot rollout",
    stage_id: "s2",
    amount_minor: 1_250_000,
    company_id: "o1",
    stalled: true,
    // We wrote last, and nobody answered in ten days: the stall, on the chip.
    last_email: { occurred_at: daysAgo(10), direction: "outbound" },
  },
  // The reader may not read this one's company: the wire sends no id and names
  // the field, so the card carries the mask rather than an empty slot.
  {
    ...deal,
    id: "b3",
    name: "Northgate framework",
    company_id: null,
    masked_fields: ["company_id"],
  },
];

function installBoardStub() {
  installFetchStub({
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: boardStages,
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /deals": () =>
      jsonResponse({
        data: boardDeals,
        page: { next_cursor: null, has_more: false },
      }),
    "POST /reports/deals-by-stage": () =>
      jsonResponse({
        report: "deals-by-stage",
        plan: {},
        columns: [],
        rows: [
          {
            stage_id: "s1",
            currency: "EUR",
            deals: 7,
            raw_minor: 700_000,
            weighted_minor: 140_000,
          },
          {
            stage_id: "s2",
            currency: "EUR",
            deals: 2,
            raw_minor: 250_000,
            weighted_minor: 150_000,
          },
        ],
      }),
    "GET /views": () =>
      jsonResponse({
        data: [
          {
            id: "v1",
            resource: "deals",
            name: "Slipping this quarter",
            query: { list: { sort: "", filters: { stalled: "true" } } },
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: "o1", display_name: "Acme GmbH" }],
        page: { next_cursor: null },
      }),
    "GET /me": () =>
      jsonResponse({
        user: { id: "u-9", display_name: "Me" },
        roles: ["rep"],
        teams: [],
      }),
  });
}

export const BoardInListSurface: Story = {
  render: () => {
    installBoardStub();
    return (
      <StoryProviders>
        <DealsScreen />
      </StoryProviders>
    );
  },
};

// The same deals as rows: the mail chip is a column here, so a reader who
// switches views reads the same fact off the same field.
export const TableInListSurface: Story = {
  render: () => {
    installBoardStub();
    window.location.hash = "#/deals?view=table";
    return (
      <StoryProviders>
        <DealsScreen />
      </StoryProviders>
    );
  },
};
