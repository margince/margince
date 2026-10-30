// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LegacyLossReasonsCard, LossReasonsCard } from "./lossreasons";

// Settings › the reasons a deal is lost for, and the review that maps the
// notes of deals lost before the list existed. A retired reason stays listed
// with its switch off, because lost deals still carry its code.

function reason(
  key: string,
  label: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `reason-${key}`,
    key,
    label,
    sort_order: 10,
    active: true,
    system: true,
    deal_count: 0,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...extra,
  };
}

const REASONS = {
  data: [
    reason("price", "Price", { deal_count: 12 }),
    reason("competitor", "Chose a competitor", { deal_count: 5 }),
    reason("no_decision", "No decision", { deal_count: 3 }),
    reason("timing", "Bad timing", { active: false, deal_count: 2 }),
  ],
};

const LEGACY = {
  data: [
    {
      text_hash: "a".repeat(64),
      text: "Their quote was cheaper",
      deal_count: 9,
      mapping: {
        id: "mapping-1",
        text_hash: "a".repeat(64),
        reason_key: "price",
        decided_by: "human:1",
        decided_at: "2026-10-02T08:00:00Z",
        version: 1,
      },
    },
    { text_hash: "b".repeat(64), text: "The champion left", deal_count: 4 },
  ],
  distinct_total: 14,
};

const ADMIN = {
  custom_field: ["read", "create", "update"],
  data_task: ["read", "update"],
} as const;
const READER = { custom_field: ["read"] } as const;

function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /loss-reasons": () => jsonResponse(REASONS),
      "GET /loss-reasons/legacy": () => jsonResponse(LEGACY),
    });
    return (
      <StoryProviders>
        <LossReasonsCard />
        <LegacyLossReasonsCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Sales/Loss reasons/Reasons",
  component: LossReasonsCard,
} satisfies Meta<typeof LossReasonsCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Admin: Story = { render: story(ADMIN) };

// Reads the list and changes nothing; the legacy review is not shown.
export const Reader: Story = { render: story(READER) };

export const AdminPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ADMIN),
};

export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: story(ADMIN),
};
