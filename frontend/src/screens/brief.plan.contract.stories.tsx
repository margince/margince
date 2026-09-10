// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlanContract } from "./brief.plan.contract";
import { StoryProviders } from "./story-utils";
import type { WeeklyPlan } from "./weeklyplan.queries";

// What the week is UP AGAINST: the room the calendar has left, and the two
// things the rep wrote down about it.
//
// The capacity line has two containers and the threshold decides which: a week
// with room is one plain sentence, and a crowded one is a warning with a
// heading, because there is now something to do about it. The three frames are
// that fork plus the absence — a week no calendar reader looked at draws no line
// at all, because "nothing booked" and "nobody counted" are opposite
// instructions and zero would claim the first.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const WEEK_START = "2026-06-08";

function commitments(count: number): WeeklyPlan["commitments"] {
  return Array.from({ length: count }, (_, at) => ({
    id: `c${at}`,
    label: `Call account ${at + 1} back`,
    state: "open" as const,
    position: at + 1,
  }));
}

function plan(over: Partial<WeeklyPlan> = {}): WeeklyPlan {
  return {
    id: "p1",
    local_week_start: WEEK_START,
    status: "open",
    commitments: commitments(2),
    risks: "The Aster renewal has no signer named yet.",
    capacity_note: "Two days at the conference from Wednesday.",
    capacity: { meetings: 4, tasks: 3 },
    ...over,
  };
}

const meta: Meta<typeof PlanContract> = {
  title: "Shell/Brief plan contract",
  component: PlanContract,
};
export default meta;

type Story = StoryObj<typeof PlanContract>;

// Room for the week: seven things booked against two commitments, so the line
// is a plain sentence and the tone stays available for the week that needs it.
export const RoomInTheWeek: Story = {
  render: () => (
    <StoryProviders>
      <PlanContract plan={plan()} editable />
    </StoryProviders>
  ),
};

// Past the threshold. The warning carries its own heading over the arithmetic,
// which is the difference between a caveat a reader scans past and a claim they
// can answer — and the fork is deliberate: drawn always, the tone would stop
// meaning anything.
export const CrowdedWeek: Story = {
  render: () => (
    <StoryProviders>
      <PlanContract
        plan={plan({
          commitments: commitments(5),
          capacity: { meetings: 6, tasks: 4 },
        })}
        editable
      />
    </StoryProviders>
  ),
};

// No calendar reader composed, and an unwritten contract beside it. Nothing
// stands where the capacity line would — the panel says less rather than
// guessing — and each field says which kind of silence it is holding.
export const NothingCountedAndNothingWritten: Story = {
  render: () => (
    <StoryProviders>
      <PlanContract
        plan={plan({
          capacity: undefined,
          risks: null,
          capacity_note: "",
        })}
        editable={false}
      />
    </StoryProviders>
  ),
};
