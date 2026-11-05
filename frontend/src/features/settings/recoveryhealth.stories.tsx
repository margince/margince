// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { RecoveryHealthCard } from "./recoveryhealth";

const meta = {
  title: "Settings/Governance/System health/Restore drills",
  component: RecoveryHealthCard,
} satisfies Meta<typeof RecoveryHealthCard>;
export default meta;
type Story = StoryObj<typeof meta>;

// The seeded matrix gives `job_health:read` to admin and ops only, so a story's
// grant follows its roles and Withheld really draws the withheld state.
const READS_JOB_HEALTH = ["admin", "ops"];

function grantFor(roles: string[]): GrantSpec {
  return roles.some((role) => READS_JOB_HEALTH.includes(role))
    ? { job_health: ["read"] }
    : {};
}

function story(health: Record<string, unknown>, roles: string[] = ["admin"]) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ roles, allow: grantFor(roles) })),
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
    recovery_within_target: true,
    data_loss_within_target: true,
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
      recovery_within_target: false,
      data_loss_within_target: false,
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
      recovery_within_target: undefined,
      notes: null,
    },
  }),
};

// No drill is one line in the intro, and no empty drill row under it.
export const NeverDrilled: Story = {
  render: story({ ...TARGETS, last_drill: null }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText(/No restore drill has been recorded/);
    await expect(canvas.queryByText("Last restore drill")).toBeNull();
  },
};

export const Withheld: Story = { render: story(PASSED, ["rep"]) };

export const PassedDark: Story = {
  globals: { theme: "dark" },
  render: story(PASSED),
};

export const FailedOverTargetAtPhoneWidth: Story = {
  ...FailedOverTarget,
  tags: ["uat-phone"],
};
