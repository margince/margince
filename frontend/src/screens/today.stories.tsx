// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { jsonResponse, StoryProviders } from "./story-utils";
import { TodayScreen } from "./today";

// Today across the states a reader actually meets: a day with a decision in it,
// a day with nothing but planned work, an empty one, and a day the reader is
// not being shown all of.

type Attention = components["schemas"]["Attention"];
type MorningBrief = components["schemas"]["MorningBrief"];

const quiet: Attention = {
  as_of: "2026-08-25T09:00:00Z",
  this_morning: [],
  needs_you: [],
  planned: [],
  done_for_you: [],
  counts: { this_morning: 0, needs_you: 0, planned: 0 },
};

// The briefing lane takes its item ids from the feed and their CONTENT from the
// brief's own read, so a story that shows the lane has to stub both — one
// without the other draws an empty panel, which is a different story.
const noBrief: MorningBrief = {
  id: "run-1",
  generated_at: "2026-08-25T05:02:00Z",
  as_of: "2026-08-25T05:02:00Z",
  local_day: "2026-08-25",
  candidate_count: 0,
  items: [],
};

function stubDay(day: Attention, brief: MorningBrief) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/attention")) {
      return jsonResponse(day);
    }
    if (url.includes("/brief")) {
      return jsonResponse(brief);
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

function today(day: Attention, brief: MorningBrief = noBrief) {
  return () => {
    stubDay(day, brief);
    return (
      <StoryProviders>
        <TodayScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TodayScreen> = {
  title: "Records/Today/Screen",
  component: TodayScreen,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof TodayScreen>;

// The ordinary morning: one thing to decide, work due, and what ran overnight.
// The decision lane is the only tinted panel and the only filled button, so the
// page's one move is findable before a word of it is read.
export const AFullDay: Story = {
  render: today({
    ...quiet,
    needs_you: [
      {
        id: "dc-1",
        source: "dedupe_candidate",
        kind: "organization",
        confidence: 0.92,
        actions: ["merge"],
      },
      {
        id: "ap-1",
        source: "approval",
        kind: "send_email",
        title: "Send the follow-up to Anna Weber at Baqend",
        due_at: "2026-08-26T09:00:00Z",
        actions: ["decide"],
      },
    ],
    planned: [
      {
        id: "t-1",
        source: "task",
        title: "Call Anna about the renewal",
        due_at: "2026-08-23T10:00:00Z",
        overdue: true,
        actions: ["complete", "snooze"],
      },
      {
        id: "t-2",
        source: "task",
        title: "Prepare the Q4 numbers",
        due_at: "2026-08-25T16:00:00Z",
        overdue: false,
        actions: ["complete", "snooze"],
      },
    ],
    done_for_you: [
      {
        id: "ap-2",
        source: "approval",
        kind: "close_date_correction",
        title: "Moved the Acme close date to 27 Sep",
        occurred_at: "2026-08-25T06:12:00Z",
        actions: ["open"],
      },
    ],
    counts: { this_morning: 0, needs_you: 2, planned: 2, duplicates_open: 6 },
  }),
};

// Nothing to decide. The lead line says so rather than leaving the reader to
// infer it from three empty panels.
export const NothingToDecide: Story = {
  render: today({
    ...quiet,
    planned: [
      {
        id: "t-3",
        source: "task",
        title: "Send the Riverty deck",
        due_at: "2026-08-25T15:00:00Z",
        overdue: false,
        actions: ["complete", "snooze"],
      },
    ],
    counts: { this_morning: 0, needs_you: 0, planned: 1 },
  }),
};

// A genuinely clear day, which is a state worth designing rather than an
// accident: the reader should be able to close the page and believe it.
export const ClearDay: Story = { render: today(quiet) };

// Relationships that went quiet, and NOTHING else on the page. This is the one
// state the lane can get wrong in a way a reader would notice: nobody is
// waiting on them, so every "is anything waiting?" branch is false, and a
// headline that only asked those would print "your day is clear" directly above
// three contacts nobody has spoken to in months. The story exists to keep that
// combination on screen.
export const RelationshipsGoingQuiet: Story = {
  render: today({
    ...quiet,
    relationship_decay: [
      {
        id: "rd-1",
        source: "relationship_decay",
        title: "Dana Weiss",
        detail: "63",
        occurred_at: "2026-06-23T09:00:00Z",
        actions: [],
      },
      {
        id: "rd-2",
        source: "relationship_decay",
        title: "Ines Sommer",
        detail: "48",
        occurred_at: "2026-07-08T09:00:00Z",
        actions: [],
      },
    ],
    counts: {
      this_morning: 0,
      needs_you: 0,
      planned: 0,
      relationship_decay: 2,
    },
  }),
};

// A lane the reader may not read. Said out loud in the warn family — nothing is
// broken, they are simply not seeing everything, and a page that stayed silent
// would report a clear day it cannot actually see.
export const PartlyHidden: Story = {
  render: today({ ...quiet, lanes_omitted: ["needs_you"] }),
};

// The overnight brief, waiting. This is the lane the whole feature exists for:
// a reader opens the page and the night has already picked where to start.
// Plain rather than tinted, because the decisions panel below is the one move
// the page asks for — a briefing item is a suggestion.
export const TheMorningBriefIsWaiting: Story = {
  render: today(
    {
      ...quiet,
      this_morning: [
        {
          id: "bi-1",
          source: "brief_item",
          rank: 1,
          subject: { type: "deal", id: "deal-1" },
          actions: ["act", "set_aside", "dismiss"],
        },
      ],
      counts: { this_morning: 1, needs_you: 0, planned: 0 },
    },
    {
      ...noBrief,
      candidate_count: 4,
      items: [
        {
          id: "bi-1",
          deal_id: "deal-1",
          rank: 1,
          composite: 0.72,
          feature_vector: {
            winnability: 0.8,
            revenue: 0.61,
            timing: 0.9,
            momentum: 0.55,
            warmth: 0.74,
          },
          evidence_ids: ["ev-1", "ev-2"],
          state: "new",
        },
      ],
    },
  ),
};

// A morning the night looked at and found nothing in. The lane says so out
// loud: a quiet morning is an answer, and an empty panel with no words reads
// like a page that failed to load.
export const AQuietMorning: Story = {
  render: today({
    ...quiet,
    counts: { this_morning: 0, needs_you: 0, planned: 0 },
  }),
};

// The briefing lane withheld. Same rule as any other lane: "you may not see
// this" and "there is none" are different answers.
export const TheMorningIsHidden: Story = {
  render: today({ ...quiet, lanes_omitted: ["this_morning"] }),
};
