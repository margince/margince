// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { DecisionCardLabels } from "./decisioncard";
import {
  DecisionContent,
  DecisionEvidence,
  DraftBody,
} from "./decisioncard.content";

// What a DecisionCard draws OF the proposal, on its own.
//
// These three are the card's internals rather than primitives of their own —
// nothing outside `decisioncard.tsx` mounts them — and the frames here are the
// three readings a payload can have, which the card itself can only show one of
// at a time: words somebody is about to send, values that would move from A to
// B, and a bag of wire keys no kind declared a policy for.
//
// The assembled card is `Design System/DecisionCard`, and that is where the
// states a READER cares about live. This node exists so the payload readings
// can be compared side by side, and because a component the catalog's capture
// gate cannot render is a component nobody checks.
//
// Both themes. The evidence chips sit on `--aiLight` with `--aiText` ink and the
// clamped body fades into the card's own ground — every one of those is a
// `color-mix()` that re-resolves on the flip.
const meta: Meta = {
  title: "Design System/DecisionCard content",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      // The card's own ground, because these are read against the dashed
      // staged surface and never against the page.
      <div className="staging-card dcard" data-layout="deck">
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const LABELS: DecisionCardLabels = {
  accept: "Accept",
  edit: "Edit",
  reject: "Reject",
  expired: "This ran out of time before anyone answered it.",
  draftSubject: "Subject",
  draftBody: "Message",
  showMore: "Show the whole message",
  showLess: "Show less",
  noContent: "This proposal carries nothing to read.",
  loading: "Reading the proposal",
};

const SHORT = "Either Tuesday or Thursday works on our side.";

const LONG = `Hi Marek,

Thanks for making the time yesterday. Pulling together what we agreed: you are
taking the security questionnaire back to Anja, and we will have the revised
schedule of rates over to you before Friday so it lands ahead of the board
paper.

One thing I want to flag early — the November start date only holds if the
questionnaire comes back inside two weeks. Later than that and we are into
December, which puts the pilot across the holiday shutdown.

Say the word if you would rather we walked through the rates on a call first.

Best,
Ada`;

// A SEND-SHAPED payload: the words somebody is about to put their name on, with
// the reason above them. The reason is unlabelled on purpose — it is a sentence
// the server wrote for a person, and captioning it would frame an explanation
// as a data point.
export const ADraftedMessage: Story = {
  render: () => (
    <DecisionContent
      draft={{ subject: "Re: the two dates that work", body: SHORT }}
      diffs={[]}
      lead="Anna asked for two dates and has not had them."
      rest={[]}
      raw={false}
      labels={LABELS}
    />
  ),
};

// A FIELD CHANGE: both sides, because a proposal that showed only the new value
// would ask a reader to agree to a move they cannot see.
export const ValuesThatWouldMove: Story = {
  render: () => (
    <DecisionContent
      draft={{ subject: null, body: null }}
      diffs={[
        { field: "stage", from: "Qualified", to: "Proposal" },
        { field: "close_date", from: null, to: "30 September 2026" },
      ]}
      lead="Three signals put this deal past qualification."
      rest={[{ key: "basis", label: "Basis", value: "the buyer's own date" }]}
      raw={false}
      labels={LABELS}
    />
  ),
};

// A kind that declared NO display policy keeps the raw reading: wire keys as
// written, in the mono face so they read as identifiers rather than as captions
// somebody chose. Honest rather than lazy — half the stageable kinds carry an
// agent's tool arguments with no typed payload to describe.
export const WireKeysAsWritten: Story = {
  render: () => (
    <DecisionContent
      draft={{ subject: null, body: null }}
      diffs={[]}
      lead={null}
      rest={[
        { key: "deal_id", label: "deal_id", value: "01a05500-0000-7000-8000" },
        { key: "flags", label: "flags", value: '["unrealistic_stale"]' },
        { key: "target_version", label: "target_version", value: "4" },
      ]}
      raw
      labels={LABELS}
    />
  ),
};

// The clamp and its expander. A `.link-button` rather than a disclosure: the
// reader gets the opening lines unasked, and the control only lifts the clamp —
// a draft nobody can see any of is a question with the answer removed.
export const AClampedBody: Story = {
  render: () => <DraftBody body={LONG} labels={LABELS} />,
};

// No words for the toggle, so the body stays clamped. The pair is optional only
// for a surface offering another way to the whole text, and an unnamed control
// is one nobody can act on.
export const AClampedBodyWithNoExpander: Story = {
  render: () => (
    <DraftBody
      body={LONG}
      labels={{ ...LABELS, showMore: undefined, showLess: undefined }}
    />
  ),
};

// THE RECEIPTS, open: this is the one surface where a person has to be able to
// check a claim before agreeing to it, so the deck draws them where there is
// room to read them.
export const EvidenceOpen: Story = {
  render: () => (
    <DecisionEvidence
      collapsed={false}
      evidence={[
        {
          evidence_snippet: "…shall we sync next week?…",
          source_type: "activity",
          source_lines: [12, 13, 14],
        },
        {
          evidence_snippet: "…the board paper goes out on the 30th…",
          source_type: "activity",
        },
      ]}
    />
  ),
};

// Collapsed, which is the row's reading: a queue of verbatim snippets buries the
// verbs, so the chip keeps its source and gives up the quotation.
export const EvidenceCollapsed: Story = {
  render: () => (
    <DecisionEvidence
      collapsed
      evidence={[
        {
          evidence_snippet: "…shall we sync next week?…",
          source_type: "activity",
          source_lines: [12, 13, 14],
        },
      ]}
    />
  ),
};

// An evidence row whose snippet came back EMPTY draws no chip. The field is
// required on the wire, so a blank one is a real answer — the source recorded
// nothing quotable — and a chip drawn over it would read as a receipt a reader
// could check.
export const NoEvidenceToShow: Story = {
  render: () => (
    <DecisionEvidence
      collapsed={false}
      evidence={[{ evidence_snippet: "", source_type: "activity" }]}
    />
  ),
};
