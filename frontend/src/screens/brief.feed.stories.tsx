// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { BriefFeed } from "./brief.feed";
import {
  leadRow,
  meetingRow,
  overnightRow,
  type Worklist,
} from "./brief.fixtures";
import { StoryProviders } from "./story-utils";

// TODAY — the panel's own states, the ones the redesign added.
//
// The whole-page frames are in `brief.stories.tsx` and the assembled parts in
// `brief.parts.stories.tsx`; what is here is what this panel decides for itself:
// whether its group headings say anything, what its count line reports, and the
// badge in its header band for what has moved since the brief was cut.
//
// Read every frame in BOTH themes with the toolbar's Theme control. The panel's
// band, its hairlines and the badge's fill are all `color-mix()` over canonical
// tokens, so a header that reads on paper can go flat on the dark ground.

const meta: Meta<typeof BriefFeed> = {
  title: "Shell/Brief today",
  component: BriefFeed,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof BriefFeed>;

type FeedItem = components["schemas"]["WorklistItem"];

/** One row, labelled with the section the server put it in. */
function sectioned(
  item: FeedItem,
  section: NonNullable<FeedItem["brief_section"]>,
): FeedItem {
  return { ...item, brief_section: section };
}

function day(queue: FeedItem[], urgent: number): Worklist {
  return {
    as_of: "2026-09-03T06:42:00Z",
    scope: "mine",
    scope_options: ["mine"],
    summary: { urgent, due: 1, lower_priority: 0, total: queue.length },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 1,
      prospecting: 1,
      review: 0,
      more_available: false,
    },
    queue,
  };
}

const MIXED = day(
  [
    sectioned(leadRow("lead-1"), "respond_now"),
    sectioned(meetingRow("meet-1", false), "prepare_conversations"),
    sectioned(overnightRow("deal-1", "d-1"), "move_revenue"),
    sectioned(leadRow("lead-2", "2026-09-04T09:00:00Z"), "build_pipeline"),
  ],
  2,
);

const ONE_GROUP = day(
  [
    sectioned(overnightRow("deal-1", "d-1"), "review_and_repair"),
    sectioned(overnightRow("deal-2", "d-2"), "review_and_repair"),
    sectioned(overnightRow("deal-3", "d-3"), "review_and_repair"),
  ],
  0,
);

// Four parts of the morning on screen, so the group headings earn their place:
// each says which part the reader has reached, and the count line under the
// title says how much is waiting and how much of it is urgent.
export const GroupsShown: Story = {
  args: { day: MIXED, state: "ready" },
};

// The SAME panel with every row in one part. The headings are gone — a label
// that is true of everything under it names the panel a second time and tells a
// reader nothing about where they are.
export const OneGroupNoHeadings: Story = {
  args: { day: ONE_GROUP, state: "ready" },
};

// What has MOVED since the brief was cut, in the header band, as the way
// through to the rows that moved. The badge is the page's claim rather than the
// panel's: absent, this panel says nothing about it.
export const ChangedSinceTheBrief: Story = {
  args: {
    day: MIXED,
    state: "ready",
    changed: { count: 4, href: "#/worklist?filter=changed_since_brief" },
  },
};

// A clear morning. The panel says so in one sentence — and it is the only
// sentence allowed to, because a read that has NOT landed must never wear it.
export const Clear: Story = {
  args: { day: day([], 0), state: "ready" },
};

export const Loading: Story = {
  args: { day: undefined, state: "loading" },
};

// The read failed. No count line, no "nothing is waiting": a rep who was told
// their morning was clear over a failed read would close the page.
export const ReadFailed: Story = {
  args: { day: undefined, state: "failed" },
};
