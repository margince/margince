// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AutomaticWorkHealthCard } from "./automaticworkhealth";

const meta = {
  title: "Settings/Governance/System health/Automatic work",
  component: AutomaticWorkHealthCard,
} satisfies Meta<typeof AutomaticWorkHealthCard>;
export default meta;
type Story = StoryObj<typeof meta>;

function story(health: Record<string, unknown>, roles: string[] = ["admin"]) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ roles, allow: { job_health: ["read"] } })),
      "GET /admin/automatic-work-health": () => jsonResponse(health),
    });
    return (
      <StoryProviders>
        <AutomaticWorkHealthCard />
      </StoryProviders>
    );
  };
}

const ZERO = {
  created: 0,
  not_mine: 0,
  rerouted: 0,
  dismissed: 0,
  rejected: 0,
  completed: 0,
  expired: 0,
  open: 0,
};

function tally(
  kind: string,
  counts: Partial<typeof ZERO>,
  above = false,
  share: number | null = null,
) {
  const full = { ...ZERO, ...counts };
  return {
    kind,
    counts: full,
    not_mine_share: share,
    small_sample: full.created < 10,
    above_threshold: above,
    by_day: full.created > 0 ? [{ date: "2026-10-20", counts: full }] : [],
  };
}

const EMPTY = {
  generated_at: "2026-10-22T09:00:00Z",
  days: 7,
  first_day: "2026-10-16",
  last_day: "2026-10-22",
  share_threshold: 0.1,
  small_sample_below: 10,
  kinds: [],
  colleagues: [],
  unattributed: 0,
};

const BUSY = {
  ...EMPTY,
  unattributed: 1,
  colleagues: [
    {
      user_id: "00000000-0000-7000-8000-000000000001",
      display_name: "Ada Rep",
      status: "active",
      waiting_not_mine: 3,
      tallies: [
        tally(
          "promise_task",
          { created: 12, not_mine: 3, completed: 6, open: 3 },
          true,
          0.25,
        ),
        tally(
          "reply_reminder",
          { created: 4, not_mine: 1, open: 3 },
          false,
          0.25,
        ),
        tally("approval_request", { created: 5, rejected: 2, completed: 3 }),
      ],
    },
    {
      user_id: "00000000-0000-7000-8000-000000000002",
      display_name: "Former Rep",
      status: "deactivated",
      waiting_not_mine: 0,
      tallies: [
        tally(
          "promise_task",
          { created: 2, rerouted: 1, expired: 1 },
          false,
          0,
        ),
        tally("reply_reminder", {}),
        tally("approval_request", {}),
      ],
    },
  ],
};

export const Busy: Story = {
  render: story(BUSY),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("25%, needs a look");
    await expect(canvas.getByText("Former Rep (not active)")).toBeTruthy();
  },
};

export const NothingHandedOut: Story = { render: story(EMPTY) };

export const Withheld: Story = { render: story(BUSY, ["rep"]) };

export const BusyDark: Story = {
  globals: { theme: "dark" },
  render: story(BUSY),
};

export const BusyAtPhoneWidth: Story = {
  ...Busy,
  tags: ["uat-phone"],
};
