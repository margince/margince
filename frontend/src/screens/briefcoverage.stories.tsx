// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BriefCoverage } from "./briefcoverage";
import { readingsDay } from "./home.fixtures";
import { StoryProviders } from "./story-utils";
import type { Worklist } from "./worklist.queries";

// What Home is NOT showing, per source.
//
// The panel draws nothing at all on a morning where every source answered and
// none was bounded, which is the ordinary morning — so it is absent from
// `home.stories.tsx` and had no frame of its own anywhere. The three days below
// are the three it exists for, and they are three DIFFERENT claims: a source
// the reader may never see, a source the page read only to its bound, and both
// at once.
//
// The split is the thing to look at, and it is a claim about who has to act. A
// refusal sits OUTSIDE the disclosure because no amount of clicking will reveal
// what it withheld; a bounded source is a detail behind a summary, because the
// page did read it and there is simply more behind. Read each frame with the
// disclosure SHUT first: what a reader meets before they click is the whole
// argument for drawing the two differently.
//
// There is deliberately no frame for the silent day. It renders null, and a
// story whose root stays empty fails the render gate rather than documenting
// the restraint — `briefcoverage.test.tsx` is where that case is held.

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
  title: "Shell/Home coverage",
  component: BriefCoverage,
};
export default meta;

type Story = StoryObj<typeof BriefCoverage>;

// Every source answered, two of them only as far as their bound. Nothing is
// stated in the open: the summary names what is behind it rather than restating
// the caveat, because the readings strip directly below carries that sentence
// on its own floor slot, where it qualifies the figures it is about.
export const BoundedSources: Story = {
  render: () => (
    <StoryProviders>
      <BriefCoverage day={day({ reach: boundedReach })} />
    </StoryProviders>
  ),
};

// The refusals, with no disclosure drawn at all — the panel is two sentences
// and there is nothing to expand. A reader is being shown less than the product
// knows, and that has to be legible without a click.
export const UnavailableSources: Story = {
  render: () => (
    <StoryProviders>
      <BriefCoverage day={day({ sources_unavailable: missingSources })} />
    </StoryProviders>
  ),
};

// Both, which is where the ordering earns its keep: the refusals lead, the
// summary follows them, and the two kinds of incompleteness stay tellable
// apart. Drawn as one list they read as five equally actionable facts.
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
