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

const quiet: Attention = {
  as_of: "2026-08-25T09:00:00Z",
  needs_you: [],
  planned: [],
  done_for_you: [],
  counts: { needs_you: 0, planned: 0 },
};

function stubAttention(day: Attention) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/attention")) {
      return jsonResponse(day);
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

function today(day: Attention) {
  return () => {
    stubAttention(day);
    return (
      <StoryProviders>
        <TodayScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TodayScreen> = {
  title: "Records/Today",
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
    counts: { needs_you: 2, planned: 2, duplicates_open: 6 },
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
    counts: { needs_you: 0, planned: 1 },
  }),
};

// A genuinely clear day, which is a state worth designing rather than an
// accident: the reader should be able to close the page and believe it.
export const ClearDay: Story = { render: today(quiet) };

// A lane the reader may not read. Said out loud in the warn family — nothing is
// broken, they are simply not seeing everything, and a page that stayed silent
// would report a clear day it cannot actually see.
export const PartlyHidden: Story = {
  render: today({ ...quiet, lanes_omitted: ["needs_you"] }),
};
