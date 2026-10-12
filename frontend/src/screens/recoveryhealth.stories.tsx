// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { RecoveryHealthCard } from "./recoveryhealth";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta<typeof RecoveryHealthCard> = {
  title: "Settings/Governance/System health/Restore drills",
  component: RecoveryHealthCard,
};
export default meta;
type Story = StoryObj<typeof RecoveryHealthCard>;

function story(health: Record<string, unknown>, roles: string[] = ["admin"]) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ roles, allow: { job_health: ["read"] } })),
      "GET /admin/recovery-health": () => jsonResponse(health),
    });
    return (
      <StoryProviders>
        <RecoveryHealthCard />
      </StoryProviders>
    );
  };
}

const TARGETS = {
  generated_at: "2026-10-09T09:30:00Z",
  recovery_target_seconds: 14_400,
  data_loss_target_seconds: 3_600,
};

const PASSED = {
  ...TARGETS,
  last_drill: {
    started_at: "2026-10-01T08:00:00Z",
    finished_at: "2026-10-01T10:15:00Z",
    restored_to: "2026-10-01T07:20:00Z",
    outcome: "passed",
    operator: "Ops On Call",
    notes: "Restored copy opened; record counts matched the source.",
    recovery_seconds: 8_100,
    data_loss_seconds: 2_400,
  },
};

export const Passed: Story = { render: story(PASSED) };

// Over both targets: the reading the card exists to make visible.
export const FailedOverTarget: Story = {
  render: story({
    ...TARGETS,
    last_drill: {
      ...PASSED.last_drill,
      outcome: "failed",
      notes: "Object store sync was a day behind the database.",
      recovery_seconds: 19_800,
      data_loss_seconds: 5_400,
    },
  }),
};

export const Running: Story = {
  render: story({
    ...TARGETS,
    last_drill: {
      ...PASSED.last_drill,
      outcome: "running",
      finished_at: null,
      recovery_seconds: null,
      notes: null,
    },
  }),
};

export const NeverDrilled: Story = {
  render: story({ ...TARGETS, last_drill: null }),
};

export const Withheld: Story = { render: story(PASSED, ["rep"]) };

export const PassedDark: Story = {
  globals: { theme: "dark" },
  render: story(PASSED),
};
