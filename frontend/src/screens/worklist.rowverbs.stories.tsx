// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../design-system/atoms";
import { Panel, PanelRow } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import { rowHref } from "./worklist.copy";
import type { WorklistItem } from "./worklist.queries";
import { RowActs } from "./worklist.rowverbs";
// The line's own sheet. `.worklist-row-acts` is where its trailing alignment,
// its wrapping and its `--gapActions` interval live, so a story without it
// draws the verbs as a bare inline run on the leading edge with no gap between
// them — a picture of markup rather than of the row.
import "./worklist.row.css";

// THE LINE OF VERBS, on its own.
//
// The row's own stories draw it through `WorklistRow`, which is the right way
// to see a row and the wrong way to see this: the states worth checking here
// differ only in WHICH verbs are on the line and in what order, and a picture
// of a whole row buries that under the rank, the kind, the title and the facts.
//
// What every frame is about: the line stands on the trailing edge, the lane's
// answer is its LAST control, and the two glyphs — the reader's pin and the
// hand-off — stand among the labelled ones, because a glyph with no word beside
// it reads as a stray mark rather than as a verb.
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
  return (
    <Button small variant={answer ? "primary" : "ghost"}>
      {t(message)}
    </Button>
  );
}

function taskRow(): WorklistItem {
  return {
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
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Re: pricing for the retrofit",
    because: [],
    actions: ["open", "reply"],
    dispositions: ["snooze", "not_mine", "not_sales"],
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
    id: "01a05500-0000-7000-8000-0000000000m1",
    source: "meeting_outcome",
    category: "meetings",
    level: 1,
    consequence: "data_drifts",
    title: "Quarterly review with Turbinenbau",
    occurred_at: "2026-09-02T09:00:00Z",
    because: [],
    actions: ["decide"],
    version: 4,
  };
}

/**
 * A TASK: the way to the record, the two glyphs, then the answer.
 *
 * Open · pin · hand-off · Done. The fill and the position say the same thing
 * here, which is the easy case — and the reason the waiting row below it is the
 * one worth checking: there the answer is a ghost and only its position says so.
 *
 * Only a task carries an assignee, so this is the one lane that draws the
 * hand-off at all.
 */
export const ATaskAnsweredLast: Story = {
  args: {
    item: taskRow(),
    href: rowHref(taskRow()),
    owner: "",
    primary: <Verb message="tasks.complete" answer />,
  },
};

/**
 * A BUYER WAITING: every verb a row can carry, and the wrap.
 *
 * Draft the reply · Open · pin · Snooze ▾ · Not mine · Not a customer · Reply.
 * Seven controls is where the line WRAPS, which makes this the one frame at
 * which the order can be seen to hold — and the reason the order is what it is:
 * the answer is the end of the sentence a reader has just read, at the same x
 * on every row of the queue.
 *
 * What to look for: the answer on the trailing edge, and the two glyphs still
 * among the labelled verbs rather than opening or closing a line.
 */
export const AWaitingRowWithEveryVerb: Story = {
  args: {
    item: waitingRow(),
    href: rowHref(waitingRow()),
    owner: "",
    primary: <Verb message="compose.reply" answer />,
  },
};

/**
 * A LANE WHOSE VERBS ARE EQUAL has no answer, and nothing on the line is
 * filled.
 *
 * Held, no-show and cancelled are three records of what already happened, and
 * promoting one of them would be the product claiming an expectation it has not
 * got about a meeting it knows nothing about. They arrive as `equals` rather
 * than as `primary`, which is the whole difference: no fill, and no claim on
 * the end of the line.
 */
export const EqualVerbsHaveNoAnswer: Story = {
  args: {
    item: meetingOutcomeRow(),
    href: rowHref(meetingOutcomeRow()),
    owner: "",
    equals: (
      <>
        <Verb message="worklist.verb.meetingHeld" />
        <Verb message="worklist.verb.meetingNoShow" />
        <Verb message="worklist.verb.meetingCanceled" />
      </>
    ),
  },
};

/**
 * THE SAME LINE IN DARK, where every step this line is built from inverts.
 *
 * Every control on it but the answer is a ghost — `--pane` with a
 * `--borderSubtle` hairline — and the pin is that same box with no word in it.
 * So what tells one verb from the next, and the whole line from the panel it
 * stands on, is a hairline and an ink, both derived off a ground that in dark is
 * no longer the brightest thing on screen. Nothing about the arrangement can
 * say whether that still holds; only the frame can.
 *
 * What to look for: the answer still reads as the answer against the accent's
 * dark lift, the pin still reads as a control rather than as a mark, and the
 * hairlines still say where one verb ends and the next begins. The ORDER is the
 * light frames' business — this one is about whether the line stays legible.
 */
export const TheLineInDark: Story = {
  ...AWaitingRowWithEveryVerb,
  globals: { theme: "dark" },
};

/**
 * THE SAME LINE AT 390px, which is a different line.
 *
 * Two things change and neither is the wrapping. The three judgements fold into
 * ONE put-down menu — one tab stop rather than four 44px controls, which is
 * what kept the row under its height ceiling — so the line is the move, the way
 * to the record, the pin, that menu and the answer. And every control on it
 * rises to a 44px target, because a mis-hit here files somebody else's
 * judgement on a customer's message.
 *
 * What to look for: the answer still last and still on the trailing edge, the
 * pin still among the words. The thumb's own path to those judgements is the
 * row's story rather than this one — the gesture wraps the whole row, not this
 * line.
 *
 * The one frame drawn INSIDE the row's own box, because the target floor is the
 * ROW's rule rather than the line's: without that box the verbs would be drawn
 * here at the 36px a coarse pointer gets them to, which is not what a reader
 * meets. At this width the box costs nothing — the row is a wrapping flex line
 * and the verbs take all of it — while above it the row's kind column would
 * stand empty beside them, which is why the other three frames go without.
 */
export const TheLineOnAPhone: Story = {
  ...AWaitingRowWithEveryVerb,
  globals: { viewport: { value: "phone" } },
  decorators: [
    (Story) => (
      <div className="worklist-row">
        <Story />
      </div>
    ),
  ],
};
