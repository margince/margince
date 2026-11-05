// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { PartnerStagesCard } from "./partnerstages";

// Settings › the partner ladder. A stage partners stand on keeps its meaning,
// and a retired stage stays listed with its switch off.

function stage(
  key: string,
  label: string,
  meaning: string,
  sort: number,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `stage-${key}`,
    key,
    label,
    meaning,
    sort_order: sort,
    active: true,
    system: true,
    partner_count: 0,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...extra,
  };
}

const STAGES = {
  data: [
    stage("research", "Research", "candidate", 10, { partner_count: 4 }),
    stage("contract_signed", "Contract signed", "signed", 70, {
      partner_count: 2,
    }),
    stage("ready", "Ready", "ready", 80, { active: false }),
    stage("activated", "Activated", "active", 90),
    stage("pilot", "Pilot", "candidate", 95, { system: false }),
  ],
};

const ADMIN = { custom_field: ["read", "create", "update", "delete"] } as const;
const READER = { custom_field: ["read"] } as const;

function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /partner-stages": () => jsonResponse(STAGES),
    });
    return (
      <StoryProviders>
        <PartnerStagesCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Sales/Partners/Stages",
  component: PartnerStagesCard,
} satisfies Meta<typeof PartnerStagesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Admin: Story = { render: story(ADMIN) };

// Reads the ladder and changes nothing.
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
