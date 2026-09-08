// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";
// The row's own layout, the same way every surface that mounts these rows picks
// it up — worklist.tsx, brief.feed.tsx and worklist.hidden.tsx each import it.
// Without it the story drew the row as a stack of unstyled lines: the rank ran
// into the kind, the verbs were a single flow with no groups, and the panel the
// snooze caret opens laid its four lines out as a grid of pills. A story that
// does not carry the stylesheet is a picture of markup rather than of the
// screen, which is the one thing it exists to be.
import "./worklist.css";

// One row of the queue, standing on its own — the unit worklist.stories.tsx
// exercises through the whole screen. Covered here directly because fe-uat
// (frontend/AGENTS.md) credits a story only for the file it imports, and
// nothing else in the tree imports WorklistRow.

type WorklistItem = components["schemas"]["WorklistItem"];

function noticeItem(): WorklistItem {
  return {
    id: "n1",
    source: "notice",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: "A deal you own changed stage",
    detail: "Acme Renewal moved to a new pipeline stage.",
    because: [],
    actions: ["acknowledge"],
  };
}

function stubRow(readPending: boolean) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/notices/n1/read")) {
      return readPending
        ? new Promise(() => {})
        : new Response(null, { status: 204 });
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

const meta: Meta<typeof WorklistRow> = {
  title: "Records/Worklist/Row",
  component: WorklistRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel title="What to do next">
          <ol className="worklist-list">
            <li>
              <Story />
            </li>
          </ol>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof WorklistRow>;

const baseArgs = {
  position: 1,
  owner: "",
  selected: false,
  onSelect: () => undefined,
  onReview: () => undefined,
};

// A notice, and the one verb the row draws inline rather than as a link — see
// NoticeAcknowledge in worklist.row.tsx for why.
export const ANoticeToSettle: Story = {
  args: { ...baseArgs, item: noticeItem() },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// Pending: "Got it" is clicked and the read call never resolves.
export const ANoticeSettling: Story = {
  args: { ...baseArgs, item: noticeItem() },
  render: (args) => {
    stubRow(true);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Got it" }),
    );
  },
};

// A meeting, drawn with the clock a rep is racing.
//
// The state this row could not reach before: it said "starting shortly" whether
// the meeting began in four minutes or in fifty, so the one row that has to be
// opened BEFORE a wall-clock time was the row that would not say the time.
//
// Dated deliberately far enough out that the story is stable whenever it is
// looked at — a fixture pinned to "today" renders differently every day and the
// snapshot argues with itself.
export const AMeetingWithItsStartTime: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "m1",
      source: "meeting",
      category: "meetings",
      level: 1,
      consequence: "meeting_unprepared",
      title: "Quarterly review with Turbinenbau",
      kind: "unprepared",
      due_at: "2026-09-02T13:00:00Z",
      because: [{ kind: "meeting_soon" }, { kind: "meeting_unprepared" }],
      actions: [],
      // The way into the brief, in the shape the server actually sends it.
      //
      // The subject stays the ACTIVITY — the row is about the appointment —
      // and the person rides separately, because the brief is not a page of
      // its own: it opens as `?prep=<activity>` on somebody's record. Writing
      // this fixture with a person SUBJECT instead would draw no control at
      // all, since moveHref reads `with_person` and returns nothing without
      // it, and the story would look identical to its sibling below while
      // claiming to show the opposite.
      subject: { type: "activity", id: "m1" },
      with_person: "0199f5c0-0000-7000-8000-0000000009a1",
      move: {
        action: "open_meeting_brief",
        activity_id: "m1",
        arguments: { person_id: "0199f5c0-0000-7000-8000-0000000009a1" },
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// The same meeting with nobody recorded on it — a calendar event with only
// colleagues, or one whose attendee links this reader may not see.
//
// It sits beside its sibling because the two must LOOK different. The brief
// opens on a person's page, so a meeting naming none has nowhere to send the
// reader, and the honest row is the clock with no control under it. A story
// that showed only the happy row would let a wrong destination ship looking
// exactly like a right one — and the two states are one absent field apart.
//
// The lane cannot tell an internal meeting from one whose attendees are all
// withheld, and deliberately does not try: both mean there is no page to read
// this brief on, so both arrive here as the same absence.
export const AMeetingWithNobodyToBriefAgainst: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "m2",
      source: "meeting",
      category: "meetings",
      level: 1,
      consequence: "meeting_unprepared",
      title: "Team stand-up",
      kind: "unprepared",
      due_at: "2026-09-02T13:00:00Z",
      because: [{ kind: "meeting_soon" }],
      actions: [],
      subject: { type: "activity", id: "m2" },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A task nobody has taken, with the date it is due and how it is put down.
//
// Three states the row gained at once: the due moment drawn, "nobody owns it"
// said out loud, and the spans behind the snooze's caret — opened here, because
// a popover closed is a story that shows nothing about what it holds.
export const ATaskNobodyOwnsBeingPutDown: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000a1",
      source: "task",
      category: "tasks",
      level: 3,
      consequence: "task_slips",
      title: "Send the retrofit quote",
      due_at: "2026-09-02T15:00:00Z",
      because: [{ kind: "due_today" }, { kind: "unassigned" }],
      actions: [],
      dispositions: ["snooze", "not_mine"],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "For how long" }),
    );
  },
};

// The same row on a phone.
//
// The state the old mobile test could not see: above this breakpoint the row is
// rank, text and verbs on one line, and the verbs never yield width — so at
// 390px the title column was squeezed to a few characters while three buttons
// held their full size beside it. Here the text takes the line and the verbs
// drop below it at a real target height.
//
// Worth a story of its own rather than a note on the desktop one: this is a
// different layout, and the two are checked by looking at both.
export const ATaskOnAPhone: Story = {
  ...ATaskNobodyOwnsBeingPutDown,
  globals: { viewport: { value: "phone" } },
  play: undefined,
};

// EVERY VERB A ROW CAN CARRY, on one line, with the answer at its head.
//
// The row that has the most of them: a buyer waiting on a reply carries the
// reply itself, the way into the record, the reader's pin and the three
// judgements the server offers. Drawn because this is the state the layout was
// rebuilt for and the one a screenshot has to be checked in — seven controls
// is where the line WRAPS, so it is the only width at which the order can be
// seen to hold: the answer first, the glyph among the words, and a labelled
// button wherever the wrap falls.
//
// The snooze's chooser is OPEN, because the split control is the other half of
// the change: the press means tomorrow, the caret means "not tomorrow", and a
// closed caret is a story showing neither.
export const AWaitingBuyerWithEveryVerb: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000a1",
      source: "customer_waiting",
      category: "customer_waiting",
      level: 1,
      consequence: "buyer_waits",
      title: "Re: pricing for the retrofit",
      detail: "“Can you confirm the lead time before Friday?”",
      because: [{ kind: "waiting_days", value: { kind: "days", days: 4 } }],
      actions: ["open", "reply"],
      dispositions: ["snooze", "not_mine", "not_sales"],
      subject: {
        type: "deal",
        id: "01a05500-0000-7000-8000-0000000000bb",
        label: "Acme Expansion",
      },
      move: {
        action: "draft_reply",
        activity_id: "01a05500-0000-7000-8000-0000000000a1",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "For how long" }),
    );
  },
};

// The hand-off, opened.
//
// Closed it is a glyph, which is the whole reason it fits on a row that already
// carries six controls; opened it is the picker and its confirm, and the two
// states are checked by looking at both. Only a task can be handed on — a group
// row names no single activity to move.
export const ATaskBeingHandedOn: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000a2",
      source: "task",
      category: "tasks",
      level: 3,
      consequence: "task_slips",
      title: "Send the retrofit quote",
      due_at: "2026-09-02T15:00:00Z",
      because: [{ kind: "due_today" }],
      actions: [],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Reassign" }),
    );
  },
};

// The five sources the row could draw and no story showed.
//
// Each is drawn from the shape its own producer emits, so the story is a
// picture of what a reader actually meets rather than of a fixture invented
// here. Held by TestEverySourceHasARowStory in worklist.storycensus.test.ts,
// which fails when a source the queue can draw has no story naming it.

// A brief item: the overnight ranking's pick, with the three verbs it carries.
// The row drew a Pin button and nothing else until #4230, which is the defect
// this story would have made visible.
export const ABriefItemToAdvance: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "bi1",
      source: "brief_item",
      category: "deals_at_risk",
      level: 3,
      consequence: "deal_slips_past_close",
      title: "Advance the Northstar renewal",
      because: [],
      actions: ["act", "set_aside", "dismiss"],
      subject: {
        type: "deal",
        id: "01a05500-0000-7000-8000-0000000000bb",
        label: "Northstar",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A promise the rep made, with the words they wrote under it. The quote is the
// whole point: a commitment card has to show both when it is due and where it
// was promised.
export const APromiseWithItsEvidence: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "cc1",
      source: "conversation_claim",
      category: "tasks",
      level: 2,
      consequence: "promise_breaks",
      title: "Send the retrofit quote",
      detail: "“I’ll get the quote over to you by Thursday.”",
      because: [],
      actions: ["open"],
      subject: {
        type: "person",
        id: "01a05500-0000-7000-8000-0000000000aa",
        label: "Kirsten Vogel",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A contact who has gone quiet, and the verb that sets them aside for a month.
export const AQuietContactToSetAside: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000aa",
      source: "relationship_decay",
      category: "customer_waiting",
      level: 5,
      consequence: "deal_drifts",
      title: "Kirsten Vogel has gone quiet",
      because: [],
      actions: ["open", "dismiss"],
      subject: {
        type: "person",
        id: "01a05500-0000-7000-8000-0000000000aa",
        label: "Kirsten Vogel",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// Work a person approved that then did not run. It names what was released
// rather than what was decided: the decision stands, the effect did not.
export const AnApprovedThingThatDidNotRun: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "fa1",
      source: "failed_approval",
      category: "system",
      level: 3,
      consequence: "customer_never_received",
      title: "This was approved, but the send did not run",
      because: [],
      actions: ["open"],
      subject: {
        type: "person",
        id: "01a05500-0000-7000-8000-0000000000aa",
        label: "Kirsten Vogel",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A source that stopped answering. No verb: restoring a mailbox is an
// administrator's job on another screen, and a button here would promise a
// repair this queue cannot make.
export const AMailboxThatStopped: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "sh1",
      source: "sync_health",
      category: "system",
      level: 6,
      consequence: "data_drifts",
      title: "A mailbox stopped syncing",
      because: [],
      actions: [],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A new lead waiting on a first reply, with the clock the SLA policy is
// counting. `response_overdue` is the reason the row gives; the phrase is the
// client's to write.
export const ALeadWaitingOnAFirstReply: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "lr1",
      source: "lead_response",
      category: "leads",
      level: 2,
      consequence: "buyer_waits",
      title: "Kirsten at LOXXESS asked about pricing",
      because: [{ kind: "response_overdue" }],
      actions: ["open"],
      subject: {
        type: "lead",
        id: "01a05500-0000-7000-8000-0000000000dd",
        label: "Kirsten at LOXXESS",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A privacy request, which is on this page only so a rep is not the last to
// know it exists. It is answered in the privacy queue, and the row says so by
// carrying a destination and no verb of its own.
export const APrivacyRequestOnItsClock: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "dsr1",
      source: "dsr",
      category: "system",
      level: 1,
      consequence: "legal_deadline_missed",
      title: "A subject access request is due",
      because: [{ kind: "legal_deadline" }],
      actions: [],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A disclosure this person is owed and has not had. Same clock as the row
// above, drawn differently in the one way that matters: this row NAMES the
// person and offers `open`, because a notice case has no screen of its own and
// the disclosure is sent from that person's page.
//
// The title names the ARTICLE rather than the case state, which is what a
// reader needs to decide anything: an Art. 14 duty means the subject does not
// know we hold their data at all.
export const ADisclosureThisPersonIsOwed: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "notice1",
      source: "notice_case",
      category: "system",
      level: 1,
      consequence: "legal_deadline_missed",
      title: "An Art. 14 disclosure is due",
      subject: { type: "person", id: "01a05500-0000-7000-8000-0000000000d1" },
      because: [{ kind: "legal_deadline" }],
      actions: ["open"],
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

/**
 * A meeting that happened, and nobody has said how it went.
 *
 * The counterpart of the row above, and the differences are the point. There is
 * no `due_at`: the meeting already began, so it cannot be late, and a deadline
 * would draw an overdue mark on a row whose whole point is that it is over.
 * `occurred_at` carries when it started instead, so the row reads as a debt
 * that has been growing rather than a countdown.
 */
export const AMeetingWithNoOutcomeRecorded: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "m2",
      source: "meeting_outcome",
      category: "meetings",
      level: 1,
      consequence: "data_drifts",
      title: "Quarterly review with Turbinenbau",
      occurred_at: "2026-09-02T09:00:00Z",
      because: [{ kind: "outcome_unrecorded" }],
      actions: [],
      subject: { type: "activity", id: "11111111-1111-7111-8111-111111111111" },
    },
  },
};

// A waiting message, drawn as the MESSAGE.
//
// The row a rep meets most, and the one no story here showed: when the server
// sends `email_summary` the row's own name is the canonical email row — sender,
// subject, preview, access badge — rather than a sentence about it, and the
// facts and verbs sit around it on the same grid. Drawn with the opener,
// because the row that shows a reader a message and refuses to open it is the
// defect that mount exists to remove.
export const AWaitingBuyerAsTheMessage: Story = {
  args: {
    ...baseArgs,
    onOpenEmail: () => undefined,
    item: {
      id: "01a05500-0000-7000-8000-0000000000e1",
      source: "customer_waiting",
      category: "customer_waiting",
      level: 1,
      consequence: "buyer_waits",
      title: "Re: pricing for the retrofit",
      because: [{ kind: "waiting_days", value: { kind: "days", days: 7 } }],
      actions: ["open", "reply"],
      dispositions: ["snooze", "not_mine"],
      email_summary: {
        activity_id: "01a05500-0000-7000-8000-0000000000e1",
        subject: "Re: pricing for the retrofit",
        preview: "Can you confirm the lead time before Friday?",
        occurred_at: "2026-09-01T07:42:00Z",
        direction: "inbound",
        counterparty: "Kirsten Vogel",
        attachment_count: 1,
        move: "needs_reply",
        display_status: "team",
        version: 3,
      },
      subject: {
        type: "deal",
        id: "01a05500-0000-7000-8000-0000000000bb",
        label: "Acme Expansion",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// The same row where the reader may see LESS of it.
//
// Two limits at once, and they are different limits. The message is on a
// thread limited to named people, which the access badge says and which leaves
// the words readable; the deal it is filed against arrives with no `label`,
// because the caller may not read that record — so the row names the message
// and says nothing about the account, rather than inventing a name for it.
//
// A message whose CONTENT is not the reader's produces no waiting row at all
// (the contract says so), which is why this is the limit a queue can actually
// draw.
export const AMessageOnALimitedThread: Story = {
  args: {
    ...baseArgs,
    onOpenEmail: () => undefined,
    item: {
      id: "01a05500-0000-7000-8000-0000000000e2",
      source: "customer_waiting",
      category: "customer_waiting",
      level: 1,
      consequence: "buyer_waits",
      title: "Re: the amended terms",
      because: [{ kind: "buyer_wrote_last" }],
      actions: ["open", "reply"],
      email_summary: {
        activity_id: "01a05500-0000-7000-8000-0000000000e2",
        subject: "Re: the amended terms",
        preview: "Legal has one more question on clause 4.",
        occurred_at: "2026-09-01T15:10:00Z",
        direction: "inbound",
        counterparty: "Ana Sommer",
        attachment_count: 0,
        move: "needs_reply",
        display_status: "selected",
        version: 2,
      },
      subject: {
        type: "deal",
        id: "01a05500-0000-7000-8000-0000000000bc",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A decision, answered from the row that ranked it.
//
// The verb is the row's one filled control and it opens a drawer rather than
// drawing the card inline: the card carries evidence, a draft and three
// answers, and measured on a phone it stood 440px tall inside a row whose
// ceiling is 208. Closed, which is this row's own state — the drawer is the
// approval card's story.
export const ADecisionToAnswerOnTheRow: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000d2",
      source: "approval",
      category: "decisions",
      level: 5,
      consequence: "work_blocked",
      title: "A reply to Kirsten Vogel is waiting for you",
      kind: "send_email",
      because: [{ kind: "blocks_customer_work" }],
      actions: ["decide"],
      subject: {
        type: "person",
        id: "01a05500-0000-7000-8000-0000000000aa",
        label: "Kirsten Vogel",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// The row in hand: the one the pane beside the queue is about.
//
// Marked on its edge rather than by a fill — the first row is selected on
// arrival, so a fill would paint a block of colour across the top of every day
// rather than answering which row the reader is working on. The rank is a
// pressed button here, which is the other half of the same state.
export const TheRowInHand: Story = {
  args: { ...baseArgs, selected: true, item: noticeItem() },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// A deal going quiet, with its figures and a crowded "why here".
//
// Two states no other story here draws. The deal's FIGURES on the meta line,
// which is the concept's sharpest example: a €160,100 deal was once reduced to
// "no contact for 83 days" while the money sat on the wire the whole time. And
// the FOLD, holding the reasons past the third together with why this row beat
// the one below it — one press for a reader who disagrees with the order, and
// no line of height for the many who do not.
export const ADealAtRiskWithItsFigures: Story = {
  args: {
    ...baseArgs,
    item: {
      id: "01a05500-0000-7000-8000-0000000000da",
      source: "deal_at_risk",
      category: "deals_at_risk",
      level: 3,
      consequence: "deal_slips_past_close",
      title: "Turbinenbau retrofit is drifting",
      because: [
        { kind: "quiet_days", value: { kind: "days", days: 21 } },
        { kind: "closing_soon" },
        { kind: "no_champion" },
        {
          kind: "expected_revenue",
          value: { kind: "money", amount_minor: 16010000, currency: "EUR" },
        },
      ],
      above_next: {
        comparator: "deadline",
        mine: { kind: "date", date: "2026-09-30T09:00:00Z" },
        theirs: { kind: "date", date: "2026-11-15T09:00:00Z" },
      },
      actions: ["open"],
      dispositions: ["snooze"],
      deal: {
        amount_minor: 16010000,
        currency: "EUR",
        expected_close_date: "2026-09-30",
        quiet_days: 21,
      },
      subject: {
        type: "deal",
        id: "01a05500-0000-7000-8000-0000000000da",
        label: "Turbinenbau retrofit",
      },
    },
  },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};
