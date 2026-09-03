// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { WorklistReadings } from "./worklist.readings";

type Worklist = components["schemas"]["Worklist"];
type WorklistReadingsData = components["schemas"]["WorklistReadings"];

// What today is worth, above the queue.
//
// The frames are about which readings have a DOOR. Each card opens the cut it
// was counted over, and a reading of none opens nothing: a labelled way in to
// an empty queue teaches a reader to stop taking them. The unpriced revenue
// slot is the third case — an em dash rather than a figure, because nothing at
// risk could be priced, and therefore no claim about how many rows are behind
// it either.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

function day(readings: Partial<WorklistReadingsData> = {}): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
      ...readings,
    },
  } as Worklist;
}

function frame(readings: Partial<WorklistReadingsData> = {}) {
  return (
    <LocaleProvider initial="en">
      <WorklistReadings day={day(readings)} onFilter={() => {}} />
    </LocaleProvider>
  );
}

const meta: Meta<typeof WorklistReadings> = {
  title: "Records/Worklist readings",
  component: WorklistReadings,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof WorklistReadings>;

/** A day with work in it: every reading carries a figure, so every card carries
 *  the way into the rows it counted. */
export const AFullDay: Story = {
  render: () =>
    frame({
      revenue_at_risk_minor: 384_500_00,
      revenue_currency: "EUR",
      buyer_replies: 14,
      prospecting: 3,
      review: 27,
    }),
};

/** A day with nothing on it. All four readings still draw — a slot that
 *  vanished at zero would fold the row at a different width from one read to
 *  the next — and none of them offers a door. */
export const NothingToDo: Story = {
  render: () => frame({ revenue_at_risk_minor: 0, revenue_currency: "EUR" }),
};

/** Nothing at risk could be PRICED, which is not the same as nothing being at
 *  risk. The slot says so in words rather than drawing a nought. */
export const Unpriced: Story = {
  render: () => frame({ buyer_replies: 4, review: 9 }),
};

/** The source was read to its limit, so every figure above is a floor. The
 *  caveat belongs to the row rather than to a slot. */
export const FiguresAreFloors: Story = {
  render: () =>
    frame({
      revenue_at_risk_minor: 384_500_00,
      revenue_currency: "EUR",
      buyer_replies: 14,
      prospecting: 3,
      review: 27,
      more_available: true,
    }),
};
