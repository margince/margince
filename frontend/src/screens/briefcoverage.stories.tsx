// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { readingsDay } from "./brief.fixtures";
import { BriefCoverage } from "./briefcoverage";
import { StoryProviders } from "./story-utils";
import type { Worklist } from "./worklist.queries";

// What Brief is NOT showing, per source.
//
// ONE META LINE under the readings strip, and it renders nothing at all on a
// morning where every source answered and none was bounded — which is the
// ordinary morning, so it is absent from `brief.stories.tsx` and had no frame
// of its own anywhere. The three days below are the three it exists for, and
// they are three DIFFERENT claims: a source the reader may never see, a source
// the page read only to its bound, and both at once.
//
// What to look at is the ORDER and the register. The refusals lead, because a
// source hidden from an account is a fact about that account and nothing on
// this page will reveal what it held back; the bounds follow, because they are
// facts about this page. It all reads at the meta size, as the strip's own
// footnote rather than as a notice of its own — this was a titled callout with
// a Disclosure inside it, standing above the figures it qualified.
//
// There is deliberately no frame for the silent day. It renders null, and a
// story whose root stays empty fails the render gate rather than documenting
// the restraint — `briefcoverage.test.tsx` is where that case is held.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

// The same morning the readings strip is drawn from, so the caveat cannot drift
// from the figures it qualifies: this panel renders directly above that strip.
function day(over: Partial<Worklist>): Worklist {
  return { ...readingsDay(), ...over };
}

// Two sources stopped at their work bound, and one read to the end. The
// complete row is the point of the third entry: listing it would bury the two
// that have something behind them, so the disclosure has to leave it out.
const boundedReach: Worklist["reach"] = [
  { source: "task", considered: 200, shown: 25, more_available: true },
  { source: "approval", considered: 50, shown: 8, more_available: true },
  // Bounded, and carrying everything it counted. It is the case the copy used
  // to contradict itself on — "8 shown of at least 8 read" — and it now reads
  // exactly like the two above it, because what the line says is true of both.
  { source: "lead_response", considered: 8, shown: 8, more_available: true },
  { source: "meeting", considered: 4, shown: 4, more_available: false },
];

// Both reasons a source can be missing, because they are two different
// sentences and only one of them is anybody's to fix: `withheld` is the
// reader's own grants, `failed` is an outage.
const missingSources: Worklist["sources_unavailable"] = [
  { source: "dsr", reason: "withheld" },
  { source: "lead_response", reason: "failed" },
];

const meta: Meta<typeof BriefCoverage> = {
  title: "Shell/Brief coverage",
  component: BriefCoverage,
};
export default meta;

type Story = StoryObj<typeof BriefCoverage>;

// Every source answered, three of them only as far as their bound. What to read
// is the LINE: one lead ("Read to a limit") with every source hanging off it
// under the same separator, at the meta size, so it reads as the strip's
// footnote rather than as three notices run together. The third source carries
// everything it counted and reads like the other two, because "this much shown,
// more may exist" is true of all three.
export const BoundedSources: Story = {
  render: () => (
    <StoryProviders>
      <BriefCoverage day={day({ reach: boundedReach })} />
    </StoryProviders>
  ),
};

// The refusals alone. A reader is being shown less than the product knows, and
// that has to be legible without a press.
export const UnavailableSources: Story = {
  render: () => (
    <StoryProviders>
      <BriefCoverage day={day({ sources_unavailable: missingSources })} />
    </StoryProviders>
  ),
};

// Both, which is where the ordering earns its keep: the refusals lead and the
// bounds follow, so the two kinds of incompleteness stay tellable apart even
// on one line.
export const BoundedAndUnavailable: Story = {
  render: () => (
    <StoryProviders>
      <BriefCoverage
        day={day({
          reach: boundedReach,
          sources_unavailable: missingSources,
        })}
      />
    </StoryProviders>
  ),
};
