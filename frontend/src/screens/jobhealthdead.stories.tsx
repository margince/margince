// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeadWorkCallout } from "./jobhealthdead";
import { StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Settings/Governance/System health/Job health/Dead work",
};
export default meta;

type Story = StoryObj;

function kind(deadRecent: number) {
  return {
    kind: "capture_classify",
    queue: "default",
    fleet_wide: false,
    waiting: 0,
    running: 0,
    retrying: 0,
    dead: 531,
    dead_recent: deadRecent,
    oldest_waiting_age_seconds: null,
  };
}

function story(deadRecent: number) {
  return () => (
    <StoryProviders>
      <DeadWorkCallout
        health={{
          generated_at: "2026-08-13T09:30:00Z",
          dead_window_hours: 24,
          kinds: [kind(deadRecent)],
          recent_failures: [],
        }}
      />
    </StoryProviders>
  );
}

// One dead job reads in the singular, in both the title and the body.
export const OneDeadJob: Story = { render: story(1) };

export const SeveralDeadJobs: Story = { render: story(4) };
