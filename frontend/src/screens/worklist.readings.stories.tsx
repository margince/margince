// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StoryProviders } from "./story-utils";
import type { Worklist } from "./worklist.queries";
import { WorklistReadings } from "./worklist.readings";

// The day's four readings, in the band above the queue.
//
// What is worth looking at is the money slot, because the strip formats the
// server's figures and never computes them, and three different answers reach
// it: a priced sum, a sum in units nobody named, and a day nothing could be
// priced on. The last two are not zero — one of them used to draw as €0 — and
// the three stories below are how a reader tells them apart at a glance.
//
// The floor caveat is the other one: it belongs to the plate rather than to a
// slot, because the four are read across as one statement and marking one
// invites the reading where the other three are exact.

type Readings = components["schemas"]["WorklistReadings"];

// A day carrying nothing but the readings under test. The queue is empty on
// purpose: the strip draws `day.readings` and nothing else, and a fixture with
// rows in it would suggest the queue behind it decides something here.
function day(readings: Partial<Readings> = {}): Worklist {
  return {
    as_of: "2026-09-02T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      revenue_at_risk_minor: 384_500_00,
      revenue_currency: "EUR",
      buyer_replies: 14,
      prospecting: 3,
      review: 27,
      more_available: false,
      ...readings,
    },
  };
}

const meta: Meta<typeof WorklistReadings> = {
  title: "Records/Worklist/Readings",
  component: WorklistReadings,
  parameters: { layout: "padded" },
  // Each reading is a door into the filter pill it counted, and that pill is
  // the queue's own dial — there is no queue here for it to turn.
  args: { onLane: () => {} },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof WorklistReadings>;

/** The row as a rep meets it on an ordinary morning: money first, then tallies. */
export const TheDayAtStake: Story = { args: { day: day() } };

// A source read to its work bound, so every figure above the caveat is a floor
// rather than a total. Without the line the strip states "14 buyers waiting"
// over a scan that stopped early, which tells a rep the opposite of the truth.
export const FiguresAreFloors: Story = {
  args: { day: day({ more_available: true }) },
};

// Nothing at risk could be priced — no amount recorded, or no stored rate — so
// the slot says that instead of a number, and carries no door: the lane behind
// it is not what this reader is missing, the prices are.
export const NothingCouldBePriced: Story = {
  args: { day: day({ revenue_at_risk_minor: null }) },
};

// The amounts never went through the conversion seam, so the sum is raw minor
// units in no one currency. Formatted as euros it would read as €384,500 to a
// reader with no reason to doubt it, which is the error the seam exists to
// prevent — a figure whose units nobody knows is not money.
export const UnitsNobodyNamed: Story = {
  args: { day: day({ revenue_currency: null }) },
};

// A stored zero IS a figure, and the one this reading wants to report: the
// pipeline is safe today. It has to look different from the two absences above,
// which say nobody can tell.
export const NothingAtRisk: Story = {
  args: { day: day({ revenue_at_risk_minor: 0 }) },
};
