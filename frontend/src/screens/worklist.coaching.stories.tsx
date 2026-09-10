// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "./story-utils";
import { CoachingMoves } from "./worklist.coaching";
import type { TeamBoardMember } from "./worklist.queries";
import "./worklist.css";

// The three conversations a lead should have this morning, drawn from counts
// the board above already fetched. A section of the page rather than a notice
// on it: this is content, and every line carries the number it came from so a
// lead can check it. Nothing at all when nobody is over a threshold, which is
// why there is no empty story — an empty "no coaching needed" panel is a line
// a lead reads every morning to learn nothing.

function member(
  display_name: string,
  counts: Partial<TeamBoardMember["counts"]>,
): TeamBoardMember {
  return {
    user_id: `id-${display_name}`,
    display_name,
    counts: { waiting: 0, at_risk: 0, overdue: 0, promises_due: 0, ...counts },
  };
}

const meta: Meta<typeof CoachingMoves> = {
  title: "Records/Worklist/Coaching moves",
  component: CoachingMoves,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CoachingMoves>;

/**
 * A broken promise ahead of a bigger queue: a rep with fourteen waiting
 * customers is having a busy week, and one who broke a promise has spent
 * something the customer already counted on.
 */
export const ThreeMoves: Story = {
  render: () => (
    <StoryProviders>
      <CoachingMoves
        members={[
          member("Mara Voss", { promises_due: 4 }),
          member("Piet de Wit", { waiting: 14 }),
          member("Ana Ruiz", { overdue: 7 }),
        ]}
        onOwner={() => undefined}
      />
    </StoryProviders>
  ),
};

/** One person, named once however many thresholds they cross. */
export const OneMove: Story = {
  render: () => (
    <StoryProviders>
      <CoachingMoves
        members={[member("Mara Voss", { promises_due: 3, waiting: 20 })]}
        onOwner={() => undefined}
      />
    </StoryProviders>
  ),
};
