// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StoryProviders } from "./story-utils";
import { PairDecision } from "./worklist.pair";

// The duplicate pair, decided on the row — see worklist.pair.tsx for why the
// decision belongs here rather than behind a link.
//
// The case worth looking at is the pair that is HARD: two records whose names
// differ by capitalisation, with the same domain, where the only thing telling
// a reader which is real is how much hangs off each. That is the row a rep
// actually has to think about, and the one whose verbs have to name what they
// keep.

type WorklistItem = components["schemas"]["WorklistItem"];

const meta: Meta<typeof PairDecision> = {
  title: "Records/Worklist/Pair decision",
  component: PairDecision,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
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
export const Default: Story = { args: { item: pairRow() } };

// A record type that carries no link count — a person, where nothing hangs off
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
