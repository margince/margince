// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import type { components } from "../../api/schema";
import { project, project360 } from "../../testing/projects.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { PhaseHistoryCard } from "./projectphasehistory";

// A project's phase history: each visit with when, who and why. A move a won
// deal caused names the deal, so it never reads as a move someone chose.

type PhaseCause = components["schemas"]["Project360PhaseCause"];

const meta: Meta<typeof PhaseHistoryCard> = {
  title: "Records/Project 360/Phase history",
  component: PhaseHistoryCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PhaseHistoryCard>;

const ME = {
  actor_type: "human",
  actor_id: "human:u-me",
  actor_name: "Me",
} as const;

function history(cause?: PhaseCause, reason: string | null = null) {
  const view = project360({
    project: project({ phase: "delivering" }),
    phase_history: {
      data: [
        {
          id: "ph-1",
          from_phase: null,
          to_phase: "pursuing",
          reason: null,
          changed_at: "2026-03-01T09:00:00Z",
          changed_by: ME,
        },
        {
          id: "ph-2",
          from_phase: "pursuing",
          to_phase: "delivering",
          reason,
          changed_at: "2026-04-01T09:00:00Z",
          changed_by: ME,
          ...(cause ? { cause } : {}),
        },
      ],
      phase_durations: [
        { phase: "pursuing", seconds: 2_678_400, current: false },
        { phase: "delivering", seconds: 7_862_400, current: true },
      ],
    },
  });
  return () => (
    <StoryProviders>
      <PhaseHistoryCard view={view} />
    </StoryProviders>
  );
}

/** A move someone chose, with the reason they gave. */
export const ManualMove: Story = {
  render: history(undefined, "Contract signed"),
};

/** A won deal moved the project: the row names the deal and links to it. */
export const MovedByWin: Story = {
  render: history({
    kind: "deal_won",
    deal_id: "d-1",
    deal_name: "Phase one licence",
  }),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("link", {
      name: "Phase one licence",
    });
  },
};

/** The same move for a reader who may not see that deal. */
export const MovedByUnseenWin: Story = {
  render: history({ kind: "deal_won", deal_id: null, deal_name: null }),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/A deal was won/);
  },
};

export const MovedByWinPhone: Story = {
  tags: ["uat-phone"],
  render: history({
    kind: "deal_won",
    deal_id: "d-1",
    deal_name: "Phase one licence",
  }),
};

export const MovedByWinDark: Story = {
  globals: { theme: "dark" },
  render: history({
    kind: "deal_won",
    deal_id: "d-1",
    deal_name: "Phase one licence",
  }),
};
