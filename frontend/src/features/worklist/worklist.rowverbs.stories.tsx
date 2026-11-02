// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { Button } from "../../design-system/atoms";
import { Panel, PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { en, type MessageKey } from "../../i18n/en";
import { rowHref } from "../../lib/worklistcopy";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { WorklistItem } from "./worklist.queries";
import { WaitingReply } from "./worklist.reply";
import { RowActs } from "./worklist.rowverbs";
// The line's own sheet. `.worklist-row-acts` is where its trailing alignment,
// its wrapping and its `--gapActions` interval live, so a story without it
// draws the verbs as a bare inline run on the leading edge with no gap between
// them — a picture of markup rather than of the row.
import "./worklist.row.css";

// THE LINE OF VERBS, on its own.
//
// The row's own stories draw it through `WorklistRow`. The states worth checking
// here differ only in which verbs are on the line, and in what order. A whole
// row buries that under its rank, kind, title and facts.
//
// In every frame the line stands on the trailing edge. Done and the lane's
// answer come before the row's menu, the line's last control.
//
// Framed as a `PanelRow` and nothing else, so the line gets the width a row
// gives it without the rank and kind columns standing beside it. The row's own
// arrangement is `Records/Worklist/Row`.

// THE READER'S OWN SESSION, routed rather than left to the stub's fallback.
//
// One verb on this line asks who is holding the work: the hand-off names the
// contact a reassignment moves a task AWAY from, and on the reader's own queue
// that is whoever `/me` says. Unrouted, the stub answers a list-shaped body,
// which reads as a malformed session — every capability then fails closed and
// the frame draws a branch no story here is named for. The grants are empty on
// purpose: a rep's own queue needs none of them to offer these verbs.
function withSession() {
  installFetchStub({ "GET /me": meRoute({}, { roles: ["rep"] }) });
}

const meta: Meta<typeof RowActs> = {
  title: "Records/Worklist/Row verbs",
  component: RowActs,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      withSession();
      return (
        <StoryProviders>
          <Panel>
            <PanelRow>
              <Story />
            </PanelRow>
          </Panel>
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof RowActs>;

/**
 * One verb a LANE hands to the line, drawn the way the contract says it
 * arrives: a node the caller computes.
 *
 * `primary` and `equals` are `ReactNode`, because what answers a lane is the
 * lane's business — a task's completion writes, a meeting's outcome writes
 * somewhere else, and a reply opens a composer. None of that belongs to the
 * line. So the story hands it a button, which is what every lane hands it.
 *
 * The WORDS come from the catalog rather than being typed here: a story showing
 * a verb the product does not use is worse than no story, and these are the
 * same keys the lanes read.
 */
function Verb({
  message,
  answer,
}: Readonly<{ message: MessageKey; answer?: boolean }>) {
  const t = useT();
  return <Button variant={answer ? "primary" : "ghost"}>{t(message)}</Button>;
}

function taskRow(): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "01a05500-0000-7000-8000-0000000000t1",
    source: "task",
    category: "tasks",
    level: 3,
    consequence: "task_slips",
    title: "Send the retrofit quote",
    because: [],
    actions: ["complete", "open"],
    version: 3,
    subject: {
      type: "contact",
      id: "01a05500-0000-7000-8000-0000000000aa",
      label: "Kirsten Vogel",
    },
  };
}

function waitingRow(): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Re: pricing for the retrofit",
    because: [],
    actions: ["open", "reply"],
    dispositions: ["handled", "snooze", "not_mine", "not_sales"],
    version: 2,
    subject: {
      type: "deal",
      id: "01a05500-0000-7000-8000-0000000000bb",
      label: "Acme Expansion",
    },
    move: {
      action: "draft_reply",
      activity_id: "01a05500-0000-7000-8000-0000000000a1",
    },
  };
}

function meetingOutcomeRow(): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "01a05500-0000-7000-8000-0000000000m1",
    source: "meeting_outcome",
    category: "meetings",
    level: 1,
    consequence: "data_drifts",
    title: "Quarterly review with Turbinenbau",
    occurred_at: "2026-09-02T09:00:00Z",
    because: [],
    actions: ["decide", "cancel"],
    version: 4,
  };
}

/**
 * A task: the answer, then the row's menu.
 *
 * The fill and the position say the same thing here, which is the easy case.
 * The waiting row below is the one worth checking: there only the position
 * says which verb is the answer.
 */
export const ATaskAnsweredLast: Story = {
  args: {
    item: taskRow(),
    title: taskRow().title ?? "",
    href: rowHref(taskRow()),
    owner: "",
    titled: true,
    primary: <Verb message="tasks.complete" answer />,
  },
};

/**
 * The same task's menu, opened: the pin, then the hand-off. Only a task carries
 * an assignee, so this is the one lane whose menu offers the hand-off.
 */
export const ATasksMenu: Story = {
  ...ATaskAnsweredLast,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: en["worklist.rowMenu"].replace("{title}", taskRow().title ?? ""),
      }),
    );
  },
};

/** A task nobody holds that the reader may change: Take it stands before Done. */
export const AnUnheldTask: Story = {
  args: {
    ...ATaskAnsweredLast.args,
    item: { ...taskRow(), actions: ["complete", "snooze", "take"] },
  },
};

/**
 * A buyer waiting: the row with the most verbs, on one line.
 *
 * Done, the prepared move, Reply, then the menu. Everything else the row
 * offers is a line of that menu, so the line no longer wraps at this width.
 *
 * What to look for: Reply just before the menu, the menu on the trailing edge.
 */
export const AWaitingRowWithEveryVerb: Story = {
  args: {
    item: waitingRow(),
    title: waitingRow().title ?? "",
    href: rowHref(waitingRow()),
    owner: "",
    titled: true,
    primary: <Verb message="compose.reply" answer />,
  },
};

/** A message the reader sent that nobody answered: the follow-up is the answer. */
export const AFollowUpRow: Story = {
  args: {
    item: {
      ...waitingRow(),
      source: "awaiting_reply",
      category: "tasks",
      consequence: "none",
      title: "The proposal we discussed",
      because: [
        { kind: "you_wrote_last" },
        { kind: "no_reply_days", value: { kind: "days", days: 3 } },
      ],
    },
    title: "The proposal we discussed",
    href: rowHref(waitingRow()),
    owner: "",
    titled: true,
    primary: <Verb message="worklist.verb.draft_follow_up_now" answer />,
  },
};

/** A meeting with nothing sent since: the follow-up is a fresh message. */
export const AMeetingFollowUpRow: Story = {
  args: {
    item: {
      ...waitingRow(),
      source: "meeting_follow_up",
      category: "meetings",
      consequence: "none",
      title: "Discovery workshop",
      move: undefined,
      because: [
        { kind: "met_days_ago", value: { kind: "days", days: 3 } },
        { kind: "nothing_sent_since" },
      ],
    },
    title: "Discovery workshop",
    href: rowHref(waitingRow()),
    owner: "",
    titled: true,
    primary: <Verb message="worklist.verb.draft_follow_up_now" answer />,
  },
};

/** The same follow-up, owed to three attendees of a meeting filed on a deal. */
export const AMeetingFollowUpToThreeAttendees: Story = {
  args: {
    item: {
      ...waitingRow(),
      source: "meeting_follow_up",
      category: "meetings",
      consequence: "none",
      title: "Pricing review",
      move: undefined,
      contact: {
        id: "01a05500-0000-7000-8000-0000000000d1",
        label: "Ana Brandt",
      },
      recipients: [
        { id: "01a05500-0000-7000-8000-0000000000d1", label: "Ana Brandt" },
        { id: "01a05500-0000-7000-8000-0000000000d2", label: "Ben Ortiz" },
        { id: "01a05500-0000-7000-8000-0000000000d3", label: "Cleo Nguyen" },
      ],
      because: [
        { kind: "met_days_ago", value: { kind: "days", days: 3 } },
        { kind: "nothing_sent_since" },
      ],
    },
    title: "Pricing review",
    href: rowHref(waitingRow()),
    owner: "",
    titled: true,
    primary: <Verb message="worklist.verb.draft_follow_up_now" answer />,
  },
};

/** A follow-up with no attendee on file: the row opens the deal instead. */
export const AMeetingFollowUpWithNoAttendee: Story = {
  args: {
    item: {
      ...waitingRow(),
      source: "meeting_follow_up",
      category: "meetings",
      consequence: "none",
      title: "Pricing review",
      move: undefined,
      actions: ["open"],
      contact: undefined,
      because: [
        { kind: "met_days_ago", value: { kind: "days", days: 3 } },
        { kind: "nothing_sent_since" },
        { kind: "no_attendee_on_file" },
      ],
    },
    title: "Pricing review",
    href: rowHref(waitingRow()),
    owner: "",
    titled: true,
  },
};

/**
 * A LANE WHOSE VERBS ARE EQUAL has no answer, and nothing on the line is
 * filled.
 *
 * Saying what came of a meeting and recording that it was called off are two
 * answers of equal standing, and promoting either would be the product claiming
 * an expectation it has not got about a meeting it knows nothing about. They
 * arrive as `equals` rather than as `primary`, which is the whole difference:
 * no fill, and no claim on the end of the line.
 */
export const EqualVerbsHaveNoAnswer: Story = {
  args: {
    item: meetingOutcomeRow(),
    title: meetingOutcomeRow().title ?? "",
    href: rowHref(meetingOutcomeRow()),
    owner: "",
    titled: true,
    equals: (
      <>
        <Verb message="worklist.verb.meetingUpdate" />
        <Verb message="worklist.verb.meetingCanceled" />
      </>
    ),
  },
};

/**
 * The same line in dark, where every ghost control stands on a darker panel.
 *
 * A hairline and an ink tell one verb from the next, both derived off a ground
 * that is no longer the brightest thing on screen. Only the frame can say
 * whether the answer still reads as the answer and the menu as a control.
 */
export const TheLineInDark: Story = {
  ...AWaitingRowWithEveryVerb,
  globals: { theme: "dark" },
};

/**
 * The same line at 390px, inside the row's own box.
 *
 * Done joins the menu below 720px, so the line is the move, the answer and the
 * menu. Every control rises to a 44px target, the row's rule rather than the
 * line's, which is why this frame is drawn inside the row's box. The thumb's
 * path to the judgements is the row's story: the gesture wraps the whole row.
 */
export const TheLineOnAPhone: Story = {
  ...AWaitingRowWithEveryVerb,
  globals: { viewport: { value: "phone" } },
  // `uat-phone` is what makes the capture gate drive the browser to 390px. The
  // viewport global alone moves Storybook's own frame and not the gate's, so
  // without the tag this frame was captured at the desktop width — which draws
  // the band this story exists to show folded away.
  tags: ["uat-phone"],
  decorators: [
    (Story) => (
      <div className="worklist-row">
        <Story />
      </div>
    ),
  ],
};

/**
 * A waiting message whose row names itself with the message. Its title links
 * nothing, so the way to the deal stays a line of the menu.
 */
export const ReplyToANamedDealConversation: Story = {
  args: {
    onOpenEmail: fn(),
    item: {
      ...waitingRow(),
      subject: {
        type: "deal",
        id: "01a00000-0000-7000-8000-000000000001",
        label: "Northstar renewal",
      },
      move: {
        action: "draft_reply",
        activity_id: "01a00000-0000-7000-8000-000000000002",
      },
    },
    title: waitingRow().title ?? "",
    href: "#/deals/01a00000-0000-7000-8000-000000000001",
    owner: "",
  },
};

/**
 * The same verbs on the Brief's card in hand, in the queue's own order.
 *
 * The card names and links the record, so the way to it is withheld from the
 * menu. The card takes no pin, because its order is the queue's.
 */
export const TheLineOnTheCardInHand: Story = {
  args: {
    item: waitingRow(),
    title: waitingRow().title ?? "",
    href: rowHref(waitingRow()),
    owner: "",
    framed: true,
    allowPin: false,
    primary: <Verb message="compose.reply" answer />,
  },
};

/**
 * The answer a rule prepared, which is the reply itself and not a button
 * beside it.
 *
 * A waiting message whose move is `draft_reply` gets one marked control on the
 * line: the composer's own reply. It wears the agent's face, because a machine
 * ranked it and wrote the steer. The frames above show the bare Reply instead.
 *
 * What to look for: one marked control and not two, just before the menu.
 */
export const ThePreparedReplyIsTheAnswer: Story = {
  render: () => {
    const item = waitingRow();
    return (
      <RowActs
        item={item}
        title={item.title ?? ""}
        href={rowHref(item)}
        owner=""
        titled
        primary={
          <WaitingReply
            item={item}
            to={{ type: "deal", id: "01a05500-0000-7000-8000-0000000000bb" }}
          />
        }
      />
    );
  },
};
