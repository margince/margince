// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import type { DecisionApproval, DecisionCardLabels } from "./decisioncard";
import type { DecisionDeckItem, DecisionDeckLabels } from "./decisiondeck";
import { DeckStack } from "./decisiondeck.stack";

// THE STACK, held still.
//
// The deck's own node shows this in motion, which is the one thing a frame
// cannot: what is worth looking at HERE is the geometry a gesture moves through
// — how deep the peeked edges sit, where the count and the legend stand under
// the plate, and what the plate does when there is only one card left or a card
// is on its way out.
//
// Both themes. The peeked edges are `--aiMed` at low opacity over the page's own
// ground, which is the pair most likely to vanish on the dark surface.
const meta: Meta<typeof DeckStack> = {
  title: "Design System/DecisionDeck stack",
  component: DeckStack,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 640 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DeckStack>;

const NOW = Date.parse("2026-08-24T09:00:00.000Z");
const HOUR = 60 * 60 * 1000;

const CARD: DecisionCardLabels = {
  accept: "Accept",
  edit: "Edit",
  reject: "Reject",
  skip: "Later",
  expired: "This ran out of time before anyone answered it.",
  draftSubject: "Subject",
  draftBody: "Message",
  noContent: "This proposal carries nothing to read.",
  loading: "Reading the proposal",
};

const LABELS = {
  card: CARD,
  deckLabel: "Waiting on you",
  keys: "Arrows stage a decision: → accept · ← reject · ↑ edit · ↓ later · Enter sends",
  behind: (count: number) => `${count} more behind`,
  bundleSummary: (members: number) => `One decision · ${members} items`,
  bundleMembers: (members: number) => `Show the ${members} items`,
} as unknown as DecisionDeckLabels;

function item(seed: string): DecisionDeckItem {
  return {
    kind: "single",
    id: `ap-${seed}`,
    approval: {
      id: `0198c4f1-2b6a-7c3d-9e0f-1122334400${seed}`,
      kind: "send_email",
      status: "pending",
      proposed_by: "agent:mailroom",
      created_at: "2026-08-24T07:41:00.000Z",
      expires_at: new Date(NOW + 9 * HOUR).toISOString(),
      summary: "Send the follow-up to Anna Weber",
      proposed_change: {
        subject: "Re: the two dates that work",
        body: "Either Tuesday or Thursday works on our side.",
      },
    } as DecisionApproval,
  };
}

const THREE = [item("01"), item("02"), item("03")];

const BASE = {
  now: NOW,
  labels: LABELS,
  drag: null,
  leaving: null,
  onStage: () => undefined,
  onKeyDown: () => undefined,
  onPointerDown: () => undefined,
  onPointerMove: () => undefined,
  onPointerUp: () => undefined,
  onPointerCancel: () => undefined,
  onLeaveEnd: () => undefined,
};

// Three waiting: the live card, two peeked edges behind it, the count and the
// legend under it. The legend is DRAWN rather than hidden — a shortcut nobody
// is told about belongs to whoever wrote it.
export const AStackOfThree: Story = {
  args: { ...BASE, live: THREE[0], waiting: THREE },
};

// THE LAST CARD. No edges behind it, and no count either: there is nothing left
// to say a number about, and the plate itself already says it is the only one.
export const TheLastCard: Story = {
  args: { ...BASE, live: THREE[0], waiting: [THREE[0]] },
};

// MID-DRAG, to the right: the live card leans by one degree per 22px travelled
// and past the threshold it takes a ring in the verdict's own colour — which is
// how which direction means what is learned while the finger is still down.
export const DraggedTowardsAccept: Story = {
  args: {
    ...BASE,
    live: THREE[0],
    waiting: THREE,
    drag: { pointerId: 1, startX: 0, startY: 0, dx: 140, dy: 6 },
  },
};

// The card ON ITS WAY OUT: a blank silhouette leaving from where the hand let
// go, so nothing a reader or a test can reach is on screen twice.
export const ACardLeaving: Story = {
  args: {
    ...BASE,
    live: THREE[1],
    waiting: THREE.slice(1),
    leaving: { verdict: "accept", dx: 140, dy: 6 },
  },
};
