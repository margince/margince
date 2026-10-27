// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { formatNumber } from "../../format/format";
import { translate, translatePlural } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadBoard } from "./leadpresentation.board";

// A German board. A built-in origin reads in the reader's language, not as the
// stored English label. A column holding one lead counts it in the singular.
const meta: Meta = {
  title: "Records/Leads/Board cards",
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
  origin: "event",
  origin_label: "Event",
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
  {
    ...lead,
    id: "l-3",
    full_name: "Mara Lindqvist",
    company_name: "Ostsee Fracht",
    status: "contacted" as const,
    score: 47,
  },
];

const eventOrigin = {
  id: "00000000-0000-4000-8000-0000000000e1",
  key: "event",
  label: "Event",
  label_key: "leadOrigins.builtin.event",
  intent: "inbound",
  sort_order: 1,
  active: true,
  system: true,
  lead_count: 3,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function counted(count: number): string {
  return translatePlural("de", "lead.boardCount", count, {
    count: formatNumber(count, "de"),
  });
}

export const OriginAndCountInGerman: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-origins": () => jsonResponse({ data: [eventOrigin] }),
      "POST /reports/leads-by-status": () =>
        jsonResponse({ rows: [{ status: "promoted", leads: 2 }] }),
    });
    return (
      <StoryProviders locale="de">
        <LeadBoard
          rows={rows}
          onMoved={() => undefined}
          hasMore={false}
          loadMore={() => undefined}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const fresh = await canvas.findByRole("region", {
      name: translate("de", "lead.statusNew"),
    });
    const card = await within(fresh).findByRole("button", {
      name: /Jonas Petersen/,
    });
    await expect(
      await within(card).findByText(
        translate("de", "leadOrigins.builtin.event"),
      ),
    ).toBeInTheDocument();
    await expect(within(card).queryByText(lead.origin_label)).toBeNull();
    await expect(within(fresh).getByText(counted(1))).toBeInTheDocument();
    const contacted = canvas.getByRole("region", {
      name: translate("de", "lead.statusContacted"),
    });
    await expect(within(contacted).getByText(counted(2))).toBeInTheDocument();
  },
};
