// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import {
  BriefItemCard,
  BriefItemCardPending,
  type BriefItemLabels,
} from "./briefitem";

// The Morning-Brief card in every state the queue actually puts it in. Flip the
// Theme control on each: the recede is `Card`'s recessed ground and the bars are
// a color-mix over the secondary ink, so both move with the palette and both
// have to be looked at twice.
const meta: Meta<typeof BriefItemCard> = {
  title: "Design System/BriefItemCard",
  component: BriefItemCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof BriefItemCard>;
type BriefItem = components["schemas"]["MorningBriefItem"];

const LABELS: BriefItemLabels = {
  rank: "Rank",
  composite: "Score",
  factors: {
    winnability: "Winnability",
    revenue: "Revenue",
    timing: "Timing",
    momentum: "Momentum",
    warmth: "Warmth",
  },
  evidence: "3 sources",
  evidenceNone: "No sources",
  openDeal: "Open deal",
  act: "Act",
  snooze: "Snooze",
  dismiss: "Dismiss",
  acted: "Acted",
  dismissed: "Dismissed",
  snoozed: "Snoozed",
  resurfaces: "Back",
  previouslyDismissed: "Flagged 03.06.2026 — you dismissed it.",
  returnedWith: "It came back with activity on",
};

// Fixed instants, formatted without touching a clock or a locale database, so
// the catalog renders the same figures on every machine and in every timezone.
const formatInstant = (utcIso: string) =>
  utcIso.slice(0, 16).replace("T", " ").concat(" UTC");
const formatPercent = (fraction: number) => `${Math.round(fraction * 100)}%`;

const ITEM: BriefItem = {
  id: "3f7c1a8e-0000-4000-8000-000000000001",
  deal_id: "9b21d0c4-0000-4000-8000-000000000002",
  rank: 1,
  composite: 0.78,
  feature_vector: {
    winnability: 0.6,
    revenue: 0.94,
    timing: 0.35,
    momentum: 1,
    warmth: 0.52,
  },
  evidence_ids: [
    "1a000000-0000-4000-8000-00000000000a",
    "1b000000-0000-4000-8000-00000000000b",
    "1c000000-0000-4000-8000-00000000000c",
  ],
  state: "new",
  state_at: null,
  snoozed_until: null,
};

const handlers = {
  onOpenDeal: () => {},
  onAct: () => {},
  onDismiss: () => {},
  onSnooze: () => {},
};

const base = {
  item: ITEM,
  labels: LABELS,
  dealName: "Nordwind Logistik — fleet telematics",
  amount: "€184,000.00",
  revenueBasisNote: "Revenue normalised against the €195,000.00 workspace P90",
  formatPercent,
  formatInstant,
  ...handlers,
};

// The live card. Read the bars top to bottom: the composite is the first row of
// the same grid, so the score and the five factors it is made of sit on one
// axis and cannot look like they disagree.
export const New: Story = { args: base };

// The snooze the API has served since before any UI offered it. A deferred item
// recedes with the settled ones but keeps its verbs — a rep pulls it forward
// again — and it says WHEN it comes back, because an item deferred to nowhere in
// particular is indistinguishable from one quietly dropped.
export const Snoozed: Story = {
  args: {
    ...base,
    item: {
      ...ITEM,
      rank: 3,
      state: "snoozed",
      state_at: "2026-08-24T06:12:00Z",
      snoozed_until: "2026-08-27T06:00:00Z",
    },
  },
};

// Acted. A verdict: no verbs left, the ground drops back, and the state word is
// the one thing on the card that stays at full contrast — which is exactly what
// a blanket `opacity` took away.
export const Acted: Story = {
  args: {
    ...base,
    item: {
      ...ITEM,
      rank: 2,
      state: "acted",
      state_at: "2026-08-24T07:41:00Z",
    },
  },
};

export const Dismissed: Story = {
  args: {
    ...base,
    item: {
      ...ITEM,
      rank: 5,
      state: "dismissed",
      state_at: "2026-08-23T16:20:00Z",
    },
  },
};

// A write in flight. The pressed button keeps its label, its ink and the
// reader's focus and turns a mark; the other two are refused, because a queue
// entry takes one decision at a time.
/** A deal the rep dismissed, come back. The line above the meters says when
 *  they waved it away and what brought it back — deterministic, from the row,
 *  which is why it carries no agent provenance the way a finding does. */
export const Returning: Story = {
  args: {
    ...base,
    item: {
      ...base.item,
      lineage: {
        dismissed_on: "2026-06-03",
        returned_with_activity_at: "2026-06-04T10:00:00Z",
      },
    },
  },
};

export const Acting: Story = { args: { ...base, pending: "act" } };

// The server refused the write — the item was already settled in another tab.
// The sentence is the server's, announced on a card the reader is already on.
export const Refused: Story = {
  args: {
    ...base,
    error: "This item was already acted on.",
  },
};

// Evidence-or-omit says a brief item always has sources, so ZERO is a fault
// rather than an absence, and the card says so instead of drawing an empty
// badge nobody can tell from a loading one.
export const WithoutEvidence: Story = {
  args: { ...base, item: { ...ITEM, evidence_ids: [] } },
};

// The deal is not read yet, and there is no figure to report. The link still
// works — a brief item always knows which deal it is about — and the money line
// is simply absent rather than invented.
export const DealNotReadYet: Story = {
  args: {
    ...base,
    dealName: undefined,
    amount: undefined,
    revenueBasisNote: undefined,
  },
};

// The i18n stress case: a compound noun per factor. The label column caps and
// ellipses rather than eating the tracks, so the comparison survives a
// translation nobody sized for.
export const LongLabels: Story = {
  args: {
    ...base,
    dealName:
      "Nordwind Logistik GmbH & Co. KG — Flottentelematik-Rahmenvertrag 2027",
    labels: {
      ...LABELS,
      composite: "Gesamtbewertung",
      factors: {
        winnability: "Gewinnwahrscheinlichkeit",
        revenue: "Umsatzbeitrag",
        timing: "Zeitliche Dringlichkeit",
        momentum: "Veränderung über Nacht",
        warmth: "Beziehungsstärke",
      },
    },
  },
};

// The queue while the run is being read. A card-shaped placeholder, because
// what arrives is cards: three loose bars where a card will be reflow the whole
// column the moment the run lands.
export const Loading: StoryObj<typeof BriefItemCardPending> = {
  render: () => <BriefItemCardPending label="Reading this morning's brief" />,
};

// The shape the home queue is: ranked, best first, with the settled ones still
// present so the morning reads as progress rather than deletion.
export const Queue: Story = {
  render: () => (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-2)",
      }}
    >
      <BriefItemCard {...base} />
      <BriefItemCard
        {...base}
        dealName="Hafenwerft Bremen — dock scheduling"
        amount="€61,500.00"
        item={{
          ...ITEM,
          id: "3f7c1a8e-0000-4000-8000-000000000002",
          rank: 2,
          composite: 0.54,
          feature_vector: {
            winnability: 0.45,
            revenue: 0.31,
            timing: 0.8,
            momentum: 0.4,
            warmth: 0.66,
          },
        }}
      />
      <BriefItemCard
        {...base}
        dealName="Elbe Kälte — cold-chain retrofit"
        amount="€22,000.00"
        item={{
          ...ITEM,
          id: "3f7c1a8e-0000-4000-8000-000000000003",
          rank: 3,
          composite: 0.41,
          state: "acted",
          state_at: "2026-08-24T07:41:00Z",
          feature_vector: {
            winnability: 0.3,
            revenue: 0.12,
            timing: 0.6,
            momentum: 0.4,
            warmth: 0.71,
          },
        }}
      />
    </div>
  ),
};
