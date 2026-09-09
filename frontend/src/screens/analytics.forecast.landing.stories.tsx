// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StatStrip } from "../design-system/statstrip";
import { LandingCard, SufficiencyCard } from "./analytics.forecast.landing";
import { StoryProviders } from "./story-utils";

// The two readings that answer "where does this period land, and does the
// pipeline support it?" — drawn INSIDE the stat strip, because that is the only
// place they ever appear and a card measured on its own tells you nothing about
// whether it sits in the grid.
//
// The frames are the three shapes that are not a plain figure. A landing with a
// CAVEAT carries its own heading rather than repeating the card's label beside
// it, which is what it did before; a sufficiency the server could not compute
// stands as an empty state where a card would be, because it replaces the card
// rather than remarking on it; and the read pair is here to be compared with
// both.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

type Readings = components["schemas"]["ForecastReadings"];
type Landing = NonNullable<Readings["landing"]>;
type Sufficiency = NonNullable<Readings["sufficiency"]>;

const CURRENCY = "EUR";

function landing(over: Partial<Landing> = {}): Landing {
  return {
    amount_minor: 184_000_00,
    measure: "commit_evidence",
    won_minor: 92_000_00,
    remaining_minor: 92_000_00,
    ...over,
  };
}

const meta: Meta<typeof LandingCard> = {
  title: "Records/Forecast readings",
  component: LandingCard,
};
export default meta;

type Story = StoryObj<typeof LandingCard>;

// The plain projection: won plus still to come, and nothing to qualify.
export const LandingWithoutCaveat: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <LandingCard landing={landing()} currency={CURRENCY} locale="en" />
      </StatStrip>
    </StoryProviders>
  ),
};

// A manager-call installation with no current call, so the figure fell back to
// commit evidence. The caveat is the point of the frame: it says why this is
// not the plain answer, under a heading of its OWN — sharing the card's label
// read as a second copy of the figure's name.
export const LandingWithCaveat: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <LandingCard
          landing={landing({ caveat: "call_absent" })}
          currency={CURRENCY}
          locale="en"
        />
      </StatStrip>
    </StoryProviders>
  ),
};

// Coverage read, for the comparison: a figure, its bar and the basis it was
// measured against.
export const SufficiencyRead: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <SufficiencyCard
          sufficiency={
            {
              basis: "historical_median",
              reference_landing_minor: 200_000_00,
              needed_open_minor: 320_000_00,
              current_open_minor: 240_000_00,
              coverage_bp: 7500,
            } satisfies Sufficiency
          }
          currency={CURRENCY}
          locale="en"
        />
      </StatStrip>
    </StoryProviders>
  ),
};

// Nothing to measure against. It stands WHERE the card would, so it is the
// instructional empty state and not a notice about a card that is not there —
// and it says which of the two absences this is, because "no call recorded" and
// "too few closed deals" are different things to do something about.
export const SufficiencyAbsent: Story = {
  render: () => (
    <StoryProviders>
      <StatStrip>
        <SufficiencyCard
          sufficiency={{ absent: "insufficient_basis" }}
          currency={CURRENCY}
          locale="en"
        />
      </StatStrip>
    </StoryProviders>
  ),
};
