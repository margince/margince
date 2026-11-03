// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { ListQuery } from "../../components/listquery";
import { translate } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadBoard } from "./leadpresentation";

// The board's terminal columns count from the leads-by-status report. A failed
// report says the zeros are not figures; a withheld or refused one says why.
const meta: Meta = {
  title: "Records/Leads/Board",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const UNNARROWED: ListQuery = {
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
  perPage: 25,
};

const lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  company_name: "Nordwind Logistik",
  status: "new" as const,
  score: 82,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const rows = [
  lead,
  {
    ...lead,
    id: "l-2",
    full_name: "Petra Vogel",
    company_name: "Südwind AG",
    status: "contacted" as const,
    score: 54,
  },
];

function Board({ query = UNNARROWED }: Readonly<{ query?: ListQuery }>) {
  return (
    <StoryProviders>
      <LeadBoard
        query={query}
        rows={rows}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />
    </StoryProviders>
  );
}

export const Counted: Story = {
  render: () => {
    installFetchStub({
      "POST /reports/leads-by-status": () =>
        jsonResponse({
          rows: [
            { status: "promoted", leads: 12 },
            { status: "disqualified", leads: 4 },
          ],
        }),
    });
    return <Board />;
  },
};

export const CountsUnavailable: Story = {
  render: () => {
    installFetchStub({
      "POST /reports/leads-by-status": () =>
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
    });
    return <Board />;
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("alert"),
    ).toHaveTextContent("Qualified and Disqualified counts did not load.");
  },
};

export const CountsWithheld: Story = {
  render: () => {
    installFetchStub({
      "GET /leads": () =>
        jsonResponse({
          data: [],
          page: { has_more: false, next_cursor: null },
        }),
    });
    return <Board query={{ ...UNNARROWED, filters: { min_score: "80" } }} />;
  },
  play: async ({ canvasElement }) => {
    const qualified = await within(canvasElement).findByRole("region", {
      name: translate("en", "lead.statusPromoted"),
    });
    await expect(qualified).toHaveTextContent(
      translate("en", "lead.boardCountsNoScoreFilter"),
    );
  },
};

export const CountsRefused: Story = {
  render: () => {
    installFetchStub({
      "POST /reports/leads-by-status": () =>
        jsonResponse({ title: "Forbidden", status: 403 }, 403),
    });
    return (
      <Board query={{ ...UNNARROWED, filters: { owner_id: "u-colleague" } }} />
    );
  },
  play: async ({ canvasElement }) => {
    const disqualified = await within(canvasElement).findByRole("region", {
      name: translate("en", "lead.statusDisqualified"),
    });
    await expect(
      await within(disqualified).findByText(
        translate("en", "lead.boardCountsOwnerNotMeasurable"),
      ),
    ).toBeInTheDocument();
  },
};
