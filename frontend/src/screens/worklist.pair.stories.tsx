// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Panel, PanelRow } from "../design-system/panel";
import { StoryProviders } from "./story-utils";
import { PairDecision } from "./worklist.pair";

// The duplicate pair, reviewed on the row — see worklist.pair.tsx for why the
// decision belongs here rather than behind a link.
//
// The case worth looking at is the pair that is HARD: two records whose names
// differ by capitalisation, with the same domain, where the only thing telling
// a reader which is real is how much hangs off each. That is the row a rep
// actually has to think about, and the one whose verbs have to name what they
// keep.
//
// WHAT EVERY FRAME IS ABOUT: the question leads, the two records stand side by
// side on recessed cards with the verb that keeps each one at its own foot, and
// the one answer about neither of them takes a line under both — on the
// trailing edge, and unfilled, because the two Keep verbs are the answers.
//
// Framed in a `Panel` and a `PanelRow`, which is where the review is drawn. The
// cards are `Card inset` and read as recessed AGAINST the panel's ground, so a
// frame on the bare canvas would show two boxes that are not there.

type WorklistItem = components["schemas"]["WorklistItem"];

const meta: Meta<typeof PairDecision> = {
  title: "Records/Worklist/Pair decision",
  component: PairDecision,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel>
          <PanelRow>
            <Story />
          </PanelRow>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof PairDecision>;

function pairRow(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "01a05500-0000-7000-8000-0000000000ff",
    source: "dedupe_candidate",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    because: [],
    actions: ["merge"],
    pair: {
      left: {
        id: "01a05500-0000-7000-8000-0000000000a1",
        label: "Acme GmbH",
        detail: "acme.de",
        related_count: 12,
      },
      right: {
        id: "01a05500-0000-7000-8000-0000000000a2",
        label: "ACME Gmbh",
        detail: "acme.de",
        related_count: 1,
      },
      evidence: [],
    },
    ...over,
  };
}

// Two companies that read alike. The link counts are the whole of the
// evidence, and each verb names the record it would keep.
//
// THE REVIEW AS A READER MEETS IT: the question, the two candidates side by
// side, and the trailing line under them. What to look for is that the two Keep
// verbs sit on ONE baseline — the cards stretch to the taller of the pair, and
// the verbs are pushed to the foot rather than following the last fact, so the
// choice between two records does not read as two different controls.
export const Default: Story = { args: { item: pairRow() } };

// THE SAME REVIEW AT 390px, where the comparison cannot be side by side.
//
// Two records at half a phone's width are two records nobody can read, so the
// cards STACK — and stacked they are read one after the other rather than
// against each other, which is why the link counts and the distinguishing line
// have to carry the comparison on their own. Everything else holds: the
// question leads, each verb keeps its own card's foot, and "Not the same" is
// one line on the trailing edge.
export const TheReviewOnAPhone: Story = {
  ...Default,
  globals: { viewport: { value: "phone" } },
};

/**
 * THE SAME REVIEW IN DARK, which is where the recess changes direction.
 *
 * `Card inset` is `--bgCard` and the panel around it is `--bgElevated`, and
 * those two swap places between the themes: in light the inset is the DARKER of
 * the pair and reads as a well cut into the card, in dark it is the LIGHTER one
 * and the two records read as plates lifted off the panel. So the frame worth
 * checking is not a colour but whether the pair still reads as two candidates
 * set apart from the verbs under them when the step runs the other way.
 *
 * The second thing to look at is the trailing line: "Not the same" is unfilled
 * on purpose, and dark is the theme where an unfilled ghost has the least
 * ground to stand on.
 */
export const TheReviewInDark: Story = {
  ...Default,
  globals: { theme: "dark" },
};

// A record type that carries no link count — a contact, where nothing hangs off
// either side. The reader decides on the names and the distinguishing line
// alone, and the row must not draw an empty signal in place of the missing one.
export const WithoutCounts: Story = {
  args: {
    item: pairRow({
      pair: {
        left: {
          id: "01a05500-0000-7000-8000-0000000000b1",
          label: "Katrin Seibert",
          detail: "k.seibert@acme.de",
        },
        right: {
          id: "01a05500-0000-7000-8000-0000000000b2",
          label: "Katrin Seibert-Vogel",
          detail: "katrin.seibert@acme.de",
        },
        evidence: [],
      },
    }),
  },
};

// A reader who may see only one side gets no payload and no decision — the
// lane withholds both together. The row still says a duplicate is waiting;
// this component draws nothing.
export const Withheld: Story = {
  args: { item: pairRow({ pair: undefined, actions: [] }) },
};

// Both records READ, neither of them the reader's to change: the ordinary case
// for a rep looking at a colleague's duplicates. The pair is named in full so
// they know what is waiting, and the verbs are replaced by the sentence that
// says who can settle it — rather than by buttons that would refuse.
export const NotYoursToSettle: Story = {
  args: { item: pairRow({ actions: [] }) },
};
