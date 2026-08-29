// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
import { formatNumber } from "../format/format";
import { LocaleProvider, useLocale } from "../i18n";
import { Badge } from "./atoms";
import { Callout } from "./callout";
import type { DecisionApproval, DecisionCardLabels } from "./decisioncard";
import {
  DecisionDeck,
  type DecisionDeckChips,
  type DecisionDeckItem,
  type DecisionDeckLabels,
  type DecisionSharedFacts,
  type StagedDecision,
} from "./decisiondeck";
import { AutonomyDot } from "./trust";

// The morning queue. Every frame here is about the same question: can a person
// answer a stack of irreversible decisions quickly WITHOUT the speed being what
// makes them irreversible. The tray is the answer — a verdict is staged, and the
// commit is a separate, deliberate press.
//
// Both themes, every frame. The tray carries `--shadow-pop` over `--bgElevated`
// and the peeked edges are `--aiMed` at low opacity: on the dark surface a
// shadow that reads on paper is invisible, which is exactly the case
// `tokens.css` themes the shadow for.
const meta: Meta<typeof DecisionDeck> = {
  title: "Design System/DecisionDeck",
  component: DecisionDeck,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 760 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DecisionDeck>;

const NOW = Date.parse("2026-08-24T09:00:00.000Z");
const HOUR = 60 * 60 * 1000;

const CARD_LABELS: DecisionCardLabels = {
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
};

const LABELS: DecisionDeckLabels = {
  card: CARD_LABELS,
  deckLabel: "Decisions waiting on you",
  viewLabel: "How to work the queue",
  viewDeck: "Deck",
  viewList: "List",
  keys: "→ accept · ← reject · ↑ edit · ↓ later · U undo the last · Enter commit",
  behind: (count) => `${count} more behind`,
  staged: (count) =>
    count === 1 ? "1 decision staged" : `${count} decisions staged`,
  commit: "Commit",
  unstage: "Undo the last",
  clearedTitle: "The queue is clear.",
  cleared: (count) =>
    count === 1 ? "You decided one thing." : `You decided ${count} things.`,
  // A catalog fixture, so the clock face is spelled out of the ISO string rather
  // than through Intl: a story that picked its own locale and its own zone would
  // be a second answer to two questions this product answers in one place each
  // (`format/format.ts` and `format/timezone.ts`), and the gates that hold those
  // do not exempt a story for being a story. The real caller hands this
  // formatter the reader's locale and zone.
  clearedTime: (atMs) =>
    `Finished at ${new Date(atMs).toISOString().slice(11, 16)}.`,
  empty: "Nothing is waiting on you.",
  bundleSummary: (members) => `1 decision · ${members} recipients`,
  bundleMembers: (members) => `The ${members} recipients`,
};

function approval(
  seed: number,
  over: Partial<DecisionApproval> = {},
): DecisionApproval {
  return {
    id: `0198c4f1-2b6a-7c3d-9e0f-${String(seed).padStart(12, "0")}`,
    kind: "held_draft",
    status: "pending",
    proposed_by: "agent:mailroom",
    created_at: "2026-08-24T07:41:00.000Z",
    expires_at: new Date(NOW + (seed + 2) * HOUR).toISOString(),
    summary: `An automation drafted a reply to counterparty ${seed}.`,
    confidence: 0.84,
    proposed_change: {
      subject: `Re: the ${seed === 1 ? "kickoff" : `round ${seed}`} — revised rates`,
      body: "Thanks for making the time yesterday. Pulling together what we agreed, and flagging the one date that is tight.",
    },
    evidence: [
      {
        evidence_snippet: "we would need it back inside a fortnight",
        source_type: "activity",
        source_id: `0198c3aa-7f10-7bbb-8888-${String(seed).padStart(12, "0")}`,
        source_lines: [112, 113],
      },
    ],
    ...over,
  };
}

function single(
  seed: number,
  over?: Partial<DecisionApproval>,
): DecisionDeckItem {
  const held = approval(seed, over);
  return { kind: "single", id: held.id, approval: held };
}

const BUNDLE: DecisionDeckItem = {
  kind: "bundle",
  id: "bundle-0198c3aa",
  bundleId: "0198c3aa-7f10-7bbb-9999-000000000001",
  members: Array.from({ length: 12 }, (_, index) =>
    approval(40 + index, {
      summary: `Introduce ourselves to person ${index + 1} found on the site.`,
      kind: "site_lead",
    }),
  ),
};

const MANY: readonly DecisionDeckItem[] = [
  single(1),
  single(2),
  BUNDLE,
  single(3),
  single(4, { expires_at: new Date(NOW - HOUR).toISOString() }),
  single(5),
];

// Every chip a caller draws about the ACT comes from `shared` — the facts every
// member of the item carries — and never from the member the card is drawn
// from. A fact the members disagree on is simply not in there, which is what
// `DivergentBundle` below shows.
const CHIPS = (
  _held: DecisionApproval,
  shared: DecisionSharedFacts,
): DecisionDeckChips => ({
  meta: (
    <>
      {shared.kind !== undefined && (
        <>
          <AutonomyDot tier="confirm" />
          <span className="t-small">
            {shared.kind === "site_lead"
              ? "Add a person found on the site"
              : "Send an email"}
          </span>
        </>
      )}
    </>
  ),
  provenance:
    shared.proposedBy === undefined
      ? undefined
      : { kind: "agent" as const, agent: "mailroom" },
  confidence: shared.confidence === undefined ? undefined : ("high" as const),
  aside: (
    <button type="button" className="link-button">
      Approval detail
    </button>
  ),
});

const BASE = {
  now: NOW,
  labels: LABELS,
  chips: CHIPS,
  onCommit: () => undefined,
};

// Six things to decide, one of them a twelve-recipient act. What to look at: the
// two peeked edges behind the live card, the count of what is still behind, and
// the keyboard legend — a swipe surface with no stated keyboard equivalent is a
// surface only a pointer can answer.
export const ManyWaiting: Story = {
  args: { ...BASE, items: MANY },
};

// Titled. The heading and the Deck/List toggle share one row — the deck draws
// its own header rather than a caller stacking a `SectionHeader` above it, which
// is two rows saying one thing. Untitled (every other story here) the toggle
// keeps the row to itself.
export const Titled: Story = {
  args: { ...BASE, items: MANY, title: "Waiting on you" },
};

// The last one. The peeked edges are gone and the count reads zero, so the
// reader can see the end coming rather than being surprised by it.
export const OneLeft: Story = {
  args: { ...BASE, items: [single(1)] },
};

// A lapsed proposal at the front of the queue. It offers no Accept — not from
// the buttons, and not from the arrow keys either.
export const ExpiredAtTheFront: Story = {
  args: {
    ...BASE,
    items: [
      single(9, { expires_at: new Date(NOW - HOUR).toISOString() }),
      single(2),
    ],
  },
};

// One act, twelve recipients, ONE decision. The API decides a bundle in one
// call, so rendering it as twelve questions would ask a reader to answer twelve
// times something they decided once — the members stay reachable underneath.
export const BundleCollapsed: Story = {
  args: { ...BASE, items: [BUNDLE, single(1)] },
};

// A bundle whose members do not agree. Two site reads of the same company stage
// under one bundle, so its members can name two different agents and carry two
// different readings — and the card drawn from one of them would state that
// one's kind, tier, provenance and confidence as the whole act's.
//
// What to look at: the chips that are NOT there. The count and the member list
// still are, and they are the honest answer to what this act proposed.
export const DivergentBundle: Story = {
  args: {
    ...BASE,
    items: [
      {
        kind: "bundle",
        id: "bundle-0198c3bb",
        bundleId: "0198c3aa-7f10-7bbb-9999-000000000002",
        members: [
          approval(60, {
            kind: "site_lead",
            summary: "Introduce ourselves to the head of operations.",
            proposed_by: "agent:deepread",
          }),
          approval(61, {
            kind: "site_lead",
            summary: "Introduce ourselves to the finance lead.",
            proposed_by: "agent:site-read",
            confidence: 0.42,
          }),
        ],
      },
    ],
  },
};

// The tray, mid-flight: three verdicts staged and nothing sent. This is the
// state the whole component exists for — the backend has no undo, so the tray
// IS the undo, and it holds until somebody presses commit.
export const StagedTray: Story = {
  args: { ...BASE, items: MANY },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(canvas.getAllByRole("button", { name: "Accept" })[0]);
    await user.click(canvas.getAllByRole("button", { name: "Accept" })[0]);
    await user.click(canvas.getAllByRole("button", { name: "Reject" })[0]);
  },
};

// The commit is out. Commit goes busy — it started the write — and Undo goes
// dim, because taking a verdict back while it is on the wire is not something
// the tray can honour.
export const Committing: Story = {
  args: { ...BASE, items: MANY, commitState: "sending" },
  play: StagedTray.play,
};

// The commit came back refused, and the tray STILL HOLDS the verdicts. They are
// the only copy of a person's answers; clearing them on failure would ask for
// all of them again.
export const CommitFailed: Story = {
  args: {
    ...BASE,
    items: MANY,
    commitState: "failed",
    notice: (
      <Callout tone="danger">
        Those decisions were not recorded. Nothing was sent — press commit
        again.
      </Callout>
    ),
  },
  play: StagedTray.play,
};

// Nothing was ever waiting. `empty` is the ONE state allowed to say there is
// none, which is why this frame and the cleared one below are different
// surfaces rather than one sentence with a count in it.
export const NothingWaiting: Story = {
  args: { ...BASE, items: [] },
};

// The read failed. Handed straight to `SurfaceState`, so a queue that did not
// load cannot read as a queue that is empty — and it carries a retry, because a
// `failed` with no way to try again is `unavailable` with extra words.
export const ReadFailed: Story = {
  args: {
    ...BASE,
    items: [],
    state: "failed",
    stateDetail: { onRetry: () => undefined },
  },
};

// The list view — the default for a reader who asked for reduced motion, and
// available to everyone through the toggle. Same cards, same verbs, in their row
// layout: nothing about a decision changes because of how it is being scanned.
export const ListView: Story = {
  args: { ...BASE, items: MANY },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(canvas.getByRole("button", { name: "List" }));
  },
};

// A live queue: the deck, and a parent that removes what it was handed. This is
// the wiring a screen owns — the deck stages, the screen sends, and the queue
// shrinks — and it is the only way to reach the cleared plate honestly, because
// the plate is the deck REMEMBERING what it watched leave.
function LiveQueue({
  start,
}: Readonly<{ start: readonly DecisionDeckItem[] }>) {
  const { locale } = useLocale();
  const [items, setItems] = useState(start);
  const send = (staged: readonly StagedDecision[]) => {
    const decided = new Set(staged.map((entry) => entry.id));
    setItems((held) => held.filter((item) => !decided.has(item.id)));
  };
  return (
    <>
      <Badge tone="accent">
        {formatNumber(items.length, locale)} left in the parent's list
      </Badge>
      <DecisionDeck {...BASE} items={items} onCommit={send} />
    </>
  );
}

// Worked to the end: the play function accepts the one item and commits it, and
// what is left is the plate — the count decided and the time it was finished.
// One earned moment, and the only state in this catalog that has to be reached
// rather than described.
export const Cleared: Story = {
  render: () => <LiveQueue start={[single(1)]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(canvas.getByRole("button", { name: "Accept" }));
    await user.click(canvas.getByRole("button", { name: "Commit" }));
  },
};

// The same wiring over the full queue, for driving the deck by hand — swipe the
// live card, or focus it and use the arrow keys.
export const Interactive: Story = {
  render: () => <LiveQueue start={MANY} />,
};
