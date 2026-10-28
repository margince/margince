// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { jsonResponse, StoryProviders } from "../../testing/story-utils";
import { WorklistHeader } from "./worklist.header";
import type { Worklist, WorklistScope } from "./worklist.queries";

type WorklistBuckets = components["schemas"]["WorklistBuckets"];

// The head of the day, in the three shapes a seat can give it.
//
// The one worth looking at is the header LINE: the sentence takes the room and
// the dials sit at its trailing edge, and which dial is drawn is a fact about
// the seat rather than a choice. A rep who can see only their own work gets
// neither — so the line is a sentence alone, which is why the group has to wrap
// as a unit rather than reserving space nobody fills.
//
// The owner picker replaces the scope switch instead of standing beside it: a
// named owner outranks the scope word, and the pair is a 422. `AColleaguesDay` is
// that state.
//
// `BothDialsDark` pins the dark theme, because the render gate draws a frame
// nobody flips in one theme only. The other frames take the Theme control.
// Check every frame at phone width too: under 720px the dials take the row.

const LENA = "00000000-0000-4000-8000-000000000001";

// The roster behind the owner picker. The header fetches it through
// `OwnerPicker`, so a frame that stubbed nothing would draw a dial holding only
// "My own day" and say nothing about the case this control exists for.
const roster = {
  data: [
    { id: LENA, display_name: "Lena Fischer" },
    { id: "00000000-0000-4000-8000-000000000002", display_name: "Marc Weber" },
  ],
  page: { next_cursor: null, has_more: false },
};

// The day behind every frame. Only the last two frames give it counts, which is
// what puts the completeness caption under the pills.
function day(
  scopes: readonly WorklistScope[],
  buckets?: WorklistBuckets,
): Worklist {
  return {
    as_of: "2026-09-02T09:00:00Z",
    scope: "mine",
    scope_options: [...scopes],
    queue: [],
    summary: {
      urgent: 2,
      due: 3,
      in_play: 1,
      lower_priority: 4,
      total: 10,
      buckets,
    },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: 0,
      revenue_currency: "EUR",
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
  };
}

function frame(
  scopes: readonly WorklistScope[],
  owner = "",
  buckets?: WorklistBuckets,
  shape: (page: Worklist) => Worklist = (page) => page,
  scope: WorklistScope = "mine",
) {
  globalThis.fetch = (async (): Promise<Response> =>
    jsonResponse(roster)) as typeof fetch;
  const page = shape(day(scopes, buckets));
  return (
    <StoryProviders>
      <WorklistHeader
        day={page}
        loaded={page.queue}
        scope={scope}
        filter="all"
        owner={owner}
        onScope={() => {}}
        onFilter={() => {}}
        onOwner={() => {}}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof WorklistHeader> = {
  title: "Records/Worklist/Header",
  component: WorklistHeader,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof WorklistHeader>;

/** A rep's own day: one scope, so no dial at all. The sentence has the line to
 *  itself and the cuts run under it. */
export const OneSeatOneDay: Story = {
  render: () => frame(["mine"]),
};

/** A lead's own day: the scope switch and the owner picker share the trailing
 *  edge of the sentence, each labelled, neither taking the page's width. */
export const BothDials: Story = {
  render: () => frame(["mine", "unassigned", "team", "all"]),
};

/** A colleague's day. The named owner outranks the scope word, so the switch is
 *  gone and the picker stands alone with the contact it chose on its face. */
export const AColleaguesDay: Story = {
  render: () => frame(["mine", "unassigned", "team", "all"], LENA),
};

/** The Unassigned scope, with the line under the dials that says what it
 *  lists. */
export const TheUnassignedScope: Story = {
  render: () =>
    frame(
      ["mine", "unassigned", "team", "all"],
      "",
      undefined,
      undefined,
      "unassigned",
    ),
};

/** Both dials in the dark theme, which is where this strip can go wrong on its
 *  own. Every colour in it is derived — the switch's selected segment, the
 *  field labels, the pills' counts and the completeness caption are all a
 *  `color-mix()` of a canonical token — and the dark accent lift moves them
 *  independently of the panel ground behind them, so a header that reads as one
 *  group in light can come apart here. */
export const BothDialsDark: Story = {
  globals: { theme: "dark" },
  render: () => frame(["mine", "unassigned", "team", "all"]),
};

/** A server that sends the partition: the sentence names the two panels by
 *  their headings, Today and To review. A half with nothing in it still prints
 *  its zero, so the line keeps its shape from day to day. */
export const TheSplit: Story = {
  render: () =>
    frame(["mine"], "", { urgent: 0, due_today: 3, planned: 9, review: 0 }),
};

// Eight duplicate pairs on one card beside two of thirty tasks: ten items on
// three cards, and the caption says both.
function grouped(page: Worklist): Worklist {
  return {
    ...page,
    queue: [
      {
        occurred_precision: "instant",
        id: "duplicates",
        source: "batch",
        category: "decisions",
        destination: "review",
        level: 6,
        consequence: "data_drifts",
        because: [],
        actions: [],
        batch: { key: "duplicates", count: 8 },
      },
      ...[1, 2].map((n) => ({
        id: `task-${n}`,
        occurred_precision: "instant" as const,
        source: "task" as const,
        category: "tasks" as const,
        level: 4,
        consequence: "task_slips" as const,
        title: `Send the revised quote ${n}`,
        because: [],
        actions: [],
      })),
    ],
    counts: [
      { category: "decisions", considered: 8, shown: 8, more_available: false },
      { category: "tasks", considered: 30, shown: 2, more_available: false },
    ],
  };
}

/** A folded group among partly loaded work: the caption counts items, and
 *  names the cards separately because one card stands for eight items. */
export const GroupedItems: Story = {
  render: () => frame(["mine"], "", undefined, grouped),
};

/** A cut read to its limit and a source that did not load. The caption drops
 *  the fraction, names the limited cut with its floor, and counts the gap. */
export const LimitedAndUnread: Story = {
  render: () =>
    frame(["mine"], "", undefined, (page) => ({
      ...grouped(page),
      counts: [
        {
          category: "decisions",
          considered: 200,
          shown: 8,
          more_available: true,
        },
        { category: "tasks", considered: 30, shown: 2, more_available: false },
      ],
      sources_unavailable: [
        { source: "bounce", category: "system", reason: "failed" },
      ],
    })),
};
