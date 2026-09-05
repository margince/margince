// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import {
  type DecisionApproval,
  DecisionCard,
  type DecisionCardLabels,
  DecisionStatusChip,
  type DecisionStatusLabels,
  DecisionToolChip,
} from "./decisioncard";
import { AutonomyDot } from "./trust";

// The staged-proposal card. The states drawn here are the ones the surface
// exists to keep apart: a proposal whose whole question is WORDS somebody is
// about to send, one that would move a value from A to B, one that has run out
// of time, and one that came back carrying nothing to read at all.
//
// Check every frame in BOTH themes. The urgency band is a `color-mix()` over
// `--warn` and `--danger`, and the ground under it is the staged card's
// `--aiLight` tint — every one of those re-resolves when the theme flips, so a
// band that reads clearly on paper can vanish on the dark surface.
const meta: Meta<typeof DecisionCard> = {
  title: "Design System/DecisionCard",
  component: DecisionCard,
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

type Story = StoryObj<typeof DecisionCard>;

// A fixed instant, so the countdown bands in these frames are the same every
// time the catalog is opened. Every `expires_at` below is an offset from it.
const NOW = Date.parse("2026-08-24T09:00:00.000Z");
const HOUR = 60 * 60 * 1000;

const LABELS: DecisionCardLabels = {
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

const BODY = `Hi Marek,

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

function approval(over: Partial<DecisionApproval> = {}): DecisionApproval {
  return {
    id: "0198c4f1-2b6a-7c3d-9e0f-11223344aabb",
    kind: "held_draft",
    status: "pending",
    proposed_by: "agent:mailroom",
    created_at: "2026-08-24T07:41:00.000Z",
    expires_at: new Date(NOW + 9 * HOUR).toISOString(),
    summary: "An automation drafted a reply to Marek Novak at Helvetia Rail.",
    confidence: 0.86,
    proposed_change: {
      subject: "Re: kickoff — revised rates and the November date",
      body: BODY,
      to: "marek.novak@helvetiarail.example",
      consent_purpose: "contract_performance",
    },
    evidence: [
      {
        evidence_snippet:
          "we would need the questionnaire back inside a fortnight to hold November",
        source_type: "activity",
        source_id: "0198c3aa-7f10-7bbb-8888-000000000001",
        source_lines: [112, 113, 114],
      },
    ],
    ...over,
  };
}

// The words the chips draw, in the shape a screen builds them. A story spells
// them in English because the catalog is read in English; a screen spells them
// through `t()`.
const STATUS_LABELS: DecisionStatusLabels = {
  expiresIn: (msRemaining) => `expires in ${hoursMinutes(msRemaining)}`,
  approved: "Approved",
  rejected: "Rejected",
  expired: "Expired",
};

// The catalog's own countdown rendering. A screen reads `formatCountdown` out of
// the message catalogue instead — the units are copy, which is exactly why the
// chip takes the span and not a rendered string.
function hoursMinutes(msRemaining: number): string {
  const minutes = Math.floor(msRemaining / 60000);
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

const META = (
  <>
    <AutonomyDot tier="confirm" />
    <span className="t-small">Send an email</span>
    <DecisionToolChip verb="send_email" label={(verb) => `via ${verb}`} />
  </>
);

// The chips a real surface draws on the meta line, over the card's OWN
// approval — the countdown and the card's edge read one deadline, so a frame
// where the two disagree is a bug the catalog can show.
function chips(over: DecisionApproval, decided = false) {
  return (
    <>
      {META}
      <DecisionStatusChip
        approval={over}
        decided={decided}
        now={NOW}
        labels={STATUS_LABELS}
      />
    </>
  );
}

const DETAIL_LINK = (
  <button type="button" className="link-button">
    Approval detail
  </button>
);

// The tall form: one card at a time, the whole payload on it. This is what the
// deck draws, and the frame to judge the drafted body's clamp in.
export const Deck: Story = {
  args: {
    approval: approval(),
    layout: "deck",
    now: NOW,
    labels: LABELS,
    provenance: { kind: "agent", agent: "mailroom" },
    confidence: "high",
    meta: chips(approval()),
    aside: DETAIL_LINK,
    onAccept: () => undefined,
    onEdit: () => undefined,
    onReject: () => undefined,
    onSkip: () => undefined,
  },
};

// The compact form the inbox and the six record surfaces draw. Same decision,
// same words, less of the body — and the evidence chip collapses to its source
// so a queue of these does not become a wall of quotations.
export const Row: Story = {
  args: {
    ...Deck.args,
    layout: "row",
    onSkip: undefined,
  },
};

// Under six hours: the edge takes the warn tone. The countdown badge is the
// caller's, and it reads the same thresholds — check that the two agree.
export const ExpiringSoon: Story = {
  args: {
    ...Deck.args,
    approval: approval({ expires_at: new Date(NOW + 3 * HOUR).toISOString() }),
    meta: chips(
      approval({ expires_at: new Date(NOW + 3 * HOUR).toISOString() }),
    ),
  },
};

// Under one hour. The whole outline goes hot, not just the edge — and
// deliberately not the ground, which carries the text.
export const Urgent: Story = {
  args: {
    ...Deck.args,
    approval: approval({
      expires_at: new Date(NOW + 20 * 60 * 1000).toISOString(),
    }),
    meta: chips(
      approval({ expires_at: new Date(NOW + 20 * 60 * 1000).toISOString() }),
    ),
  },
};

// Lapsed: the card recedes, says so, and offers NO Accept. The point of the
// frame is what is missing from it — a button whose only possible answer is a
// refusal is worse than no button.
export const Expired: Story = {
  args: {
    ...Deck.args,
    approval: approval({ expires_at: new Date(NOW - HOUR).toISOString() }),
    // The chip draws NOTHING here, and that is the frame's subject: the card
    // already says it ran out of time where its verbs were, and a badge
    // repeating the word one line above reads as two facts about one deadline.
    meta: chips(approval({ expires_at: new Date(NOW - HOUR).toISOString() })),
  },
};

// A field change rather than a draft: the old value struck through, the new one
// beside it, drawn by `FieldDiff` — the same primitive the record history and
// the audit log use, so one change reads one way everywhere.
export const FieldChange: Story = {
  args: {
    ...Deck.args,
    approval: approval({
      kind: "lifecycle_change",
      summary:
        "A mail from Helvetia Rail ended the contract, so this account looks like a former customer.",
      confidence: 0.62,
      proposed_change: {
        organization_id: "0198c3aa-7f10-7bbb-8888-000000000042",
        current_lifecycle: "customer",
        proposed_lifecycle: "former_customer",
        because: "the framework agreement was not renewed for 2027",
      },
    }),
    confidence: "med",
    meta: (
      <>
        <AutonomyDot tier="confirm" />
        <span className="t-small">Move an account's stage</span>
        {/* An unmapped kind gives the caller no verb, and the tool chip stays
            silent rather than naming a tool nobody could check. */}
        <DecisionToolChip verb={undefined} label={(verb) => `via ${verb}`} />
      </>
    ),
  },
};

// A kind that declares what it shows, beside the same payload without the
// declaration — the two halves of this card's reading, on one page.
//
// The payload is a real close-date correction: a deal id, two dates, a flag
// array and the sweep's own sentence. Undeclared, all five print as wire keys
// and the reader is handed the database row. Declared, the sentence leads and
// the identifiers are gone.
const CLOSE_DATE_PAYLOAD = {
  deal_id: "01a03781-9083-7565-8d65-5939ec0f3e70",
  basis:
    "deal has gone quiet; confirm it is still alive — set a real date or mark it lost",
  expected_close_date: "2026-10-01",
  previous_close_date: "2026-10-01",
  flags: ["unrealistic_stale"],
};

const CLOSE_DATE = approval({
  kind: "close_date_correction",
  summary: 'Confirm the real close date for "Riverty" (proposed 2026-10-01)',
  confidence: undefined,
  proposed_change: CLOSE_DATE_PAYLOAD,
  evidence: [],
});

const CLOSE_DATE_META = (
  <>
    <AutonomyDot tier="confirm" />
    <span className="t-small">Correct a close date</span>
  </>
);

// What `screens/approvalkind.ts` resolves for this payload, written out: the
// story shows the CARD, so it hands over finished strings the way the screen
// does rather than importing the screen's vocabulary. The date appears once
// because the sweep proposes the date the deal already carries — it keeps the
// date on a stale deal and asks a person instead of guessing a new one — and
// two captions over one value is not a comparison.
export const DeclaredFields: Story = {
  args: {
    ...Deck.args,
    approval: CLOSE_DATE,
    confidence: undefined,
    meta: CLOSE_DATE_META,
    display: [
      {
        field: "basis",
        label: "Why",
        value: CLOSE_DATE_PAYLOAD.basis,
        lead: true,
      },
      {
        field: "expected_close_date",
        label: "Proposed date",
        value: "01.10.2026",
      },
      {
        field: "flags",
        label: "What is wrong with it",
        value: "nothing has moved on it",
      },
    ],
  },
};

// The same proposal with no declaration: what every kind looked like before,
// and what the raw-args kinds still honestly look like.
export const UndeclaredFields: Story = {
  args: {
    ...Deck.args,
    approval: CLOSE_DATE,
    confidence: undefined,
    meta: CLOSE_DATE_META,
  },
};

// History, not a question: no verbs, no urgency band, and the verdict badge is
// the caller's. A decided card must not offer anything to press.
export const Decided: Story = {
  args: {
    ...Deck.args,
    layout: "row",
    approval: approval({
      status: "approved",
      decided_at: "2026-08-24T08:12:00.000Z",
    }),
    decided: true,
    meta: (
      <>
        <span className="t-small">Send an email</span>
        <DecisionStatusChip
          approval={approval({ status: "approved" })}
          decided
          now={NOW}
          labels={STATUS_LABELS}
        />
      </>
    ),
    onAccept: undefined,
    onEdit: undefined,
    onReject: undefined,
    onSkip: undefined,
  },
};

// A verdict in flight. Accept is the control that STARTED the write, so it is
// the one drawn busy; the other three are merely unavailable and stay dimmed.
export const VerdictInFlight: Story = {
  args: { ...Deck.args, pending: true },
};

// The payload came back with nothing a person can read. `empty` is the one
// state allowed to say "there is none", and the card says it in the caller's
// words rather than drawing a blank body that reads as a render fault.
export const NothingToRead: Story = {
  args: {
    ...Deck.args,
    approval: approval({ proposed_change: {}, evidence: [] }),
  },
};

// The read behind the card's CONTENT failed. The state vocabulary is
// `SurfaceState`'s and the card hands it straight through, so a decision surface
// cannot invent a tenth way of saying "this did not load". A retry for it belongs
// in `notice`, beside the verbs: the whole card is not the thing that failed, one
// section of it is, and the deck is where a failed QUEUE read is answered.
export const ContentFailed: Story = {
  args: {
    ...Deck.args,
    state: "failed",
  },
};

// A rejected decision, which is the other half of the decided pair: same chip,
// same words, the tone the verdict earns. Drawn as its own frame because the
// two verdicts are the states a reader scanning history has to tell apart at a
// glance, and one of them being right proves nothing about the other.
export const DecidedRejected: Story = {
  args: {
    ...Decided.args,
    approval: approval({
      status: "rejected",
      decided_at: "2026-08-24T08:12:00.000Z",
    }),
    meta: (
      <>
        <span className="t-small">Send an email</span>
        <DecisionStatusChip
          approval={approval({ status: "rejected" })}
          decided
          now={NOW}
          labels={STATUS_LABELS}
        />
      </>
    ),
  },
};
