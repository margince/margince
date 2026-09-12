// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import type { DecisionApproval, DecisionCardLabels } from "./decisioncard";
import type { DecisionDeckItem, DecisionDeckLabels } from "./decisiondeck";
import { DeckItemCard } from "./decisiondeck.item";

// ONE ITEM OF THE DECK, as the card that asks it.
//
// This is the one assembly the deck's two surfaces share — the dragged stack and
// the list draw the same card — so what is worth seeing here is the thing only
// this file decides: that a BUNDLE reads as one question with its members behind
// an expander, and that the dense line belongs to the list form and never to
// the stack.
//
// The queue around it is `Design System/DecisionDeck`; the card's own states are
// `Design System/DecisionCard`. This node exists because the assembly sits
// between them and neither of those frames can show it failing on its own.
//
// Both themes: the ground is `.staging-card`'s dashed `--aiMed` over
// `--aiLight`, the product's one signal that what is on a surface is PROPOSED
// rather than recorded, and both tokens re-resolve on the flip.
const meta: Meta<typeof DeckItemCard> = {
  title: "Design System/DecisionDeck item",
  component: DeckItemCard,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 720 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DeckItemCard>;

// A fixed instant, so the countdown these frames draw is the same every time
// the catalog is opened.
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
  showMore: "Show the whole message",
  showLess: "Show less",
  noContent: "This proposal carries nothing to read.",
  loading: "Reading the proposal",
};

const LABELS: DecisionDeckLabels = {
  card: CARD,
  deckLabel: "Waiting on you",
  viewLabel: "How the queue is shown",
  viewDeck: "Deck",
  viewList: "List",
  keys: "→ stages accept, ← reject, ↑ edit, ↓ later. Enter sends.",
  behind: (count) => `${count} more behind`,
  staged: (count) => `${count} decisions staged`,
  commit: "Send staged decisions",
  unstage: "Undo the last one",
  clearedTitle: "Deck clear",
  cleared: (count) => `${count} decisions sent`,
  clearedTime: () => "at 09:00",
  empty: "Nothing is waiting on you.",
  bundleSummary: (members) => `One decision · ${members} items`,
  bundleMembers: (members) => `Show the ${members} items`,
};

function approval(
  seed: string,
  over: Partial<DecisionApproval> = {},
): DecisionApproval {
  return {
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
    ...over,
  } as DecisionApproval;
}

const SINGLE: DecisionDeckItem = {
  kind: "single",
  id: "ap-1",
  approval: approval("01"),
};

const BUNDLE: DecisionDeckItem = {
  kind: "bundle",
  id: "bn-1",
  bundleId: "bn-1",
  members: [
    approval("11", { summary: "Lead from acme.example: Anna Weber" }),
    approval("12", { summary: "Lead from acme.example: Mira Osei" }),
    approval("13", { summary: "Lead from acme.example: Jonas Feld" }),
  ],
};

const BASE = {
  now: NOW,
  labels: LABELS,
  onStage: () => undefined,
};

// One proposal, in the deck's tall form: the whole payload on one plate.
export const ASingleInTheDeck: Story = {
  args: { ...BASE, item: SINGLE, layout: "deck" as const },
};

// The same proposal in the row form the list draws.
export const ASingleAsARow: Story = {
  args: { ...BASE, item: SINGLE, layout: "row" as const },
};

// A BUNDLE IS ONE DECISION. The count of what saying yes would decide is on the
// meta line and the members are behind an expander, because the API decides the
// set in one call — rendered flat it would be three answers to something the
// reader decided once.
export const ABundleInTheDeck: Story = {
  args: { ...BASE, item: BUNDLE, layout: "deck" as const },
};

export const ABundleAsARow: Story = {
  args: { ...BASE, item: BUNDLE, layout: "row" as const },
};

// THE DENSE LINE, which is the list form's alone: one line per decision with
// the proposal behind the line's own control. The words are the switch — a
// surface with no name for that control and none for its menu keeps the full
// row.
export const ARowAtListDensity: Story = {
  args: {
    ...BASE,
    item: SINGLE,
    layout: "row" as const,
    labels: {
      ...LABELS,
      compactRow: { detail: "What is being proposed", more: "Other answers" },
    },
  },
};

// The same words in the DECK form, which ignores them: a tall card that hid its
// proposal would be the one thing a whole plate exists not to do.
export const TheDeckIgnoresTheDensity: Story = {
  args: { ...ARowAtListDensity.args, layout: "deck" as const },
};

// A BUNDLE whose drawn member has lapsed while the rest are still answerable:
// the card shows the first member that has NOT run out of time, so its reading
// and the deck's Accept guard cannot contradict each other.
export const ABundleWhoseOldestLapsed: Story = {
  args: {
    ...BASE,
    layout: "deck" as const,
    item: {
      kind: "bundle",
      id: "bn-2",
      bundleId: "bn-2",
      members: [
        approval("21", {
          summary: "Lead from acme.example: ran out of time",
          expires_at: new Date(NOW - HOUR).toISOString(),
        }),
        approval("22", { summary: "Lead from acme.example: Mira Osei" }),
      ],
    },
  },
};
