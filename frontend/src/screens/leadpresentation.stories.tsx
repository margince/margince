// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LeadBoard } from "./leadpresentation";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The board's terminal columns count from the leads-by-status report, so a
// board with a failed report must say the zeros are not figures.
const meta: Meta = {
  title: "Records/Leads/Board",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

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

function Board() {
  return (
    <StoryProviders>
      <LeadBoard
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
    ).toHaveTextContent(
      "Qualified and Disqualified counts did not load.",
    );
  },
};
