// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";

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
// said out loud, and the spans behind "For how long" — opened here, because a
// popover closed is a story that shows nothing about what it holds.
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
