// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistHeader } from "./worklist.header";
import type { Worklist, WorklistScope } from "./worklist.queries";

// The head of the day, in the three shapes a seat can give it.
//
// The one worth looking at is the header LINE: the sentence takes the room and
// the dials sit at its trailing edge, and which dial is drawn is a fact about
// the seat rather than a choice. A rep who can see only their own work gets
// neither — so the line is a sentence alone, which is why the group has to wrap
// as a unit rather than reserving space nobody fills.
//
// The owner picker replaces the scope switch instead of standing beside it: a
// named owner outranks the scope word, and the pair is a 422. The last frame is
// that state.
//
// The last frame PINS the dark theme rather than leaving both themes to the
// toolbar's Theme control: a frame nobody flips is a frame the render gate
// draws in one theme only. The other three take the control, and every frame is
// worth a look at phone width — under 720px the dials take the row rather than
// staying capped beside a sentence that has already wrapped.

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

// A day with more behind it than the page is showing, which is what puts the
// completeness caption under the pills. Ten weighed, eight on screen.
function day(scopes: readonly WorklistScope[]): Worklist {
  return {
    as_of: "2026-09-02T09:00:00Z",
    scope: "mine",
    scope_options: [...scopes],
    queue: [],
    summary: { urgent: 2, due: 3, in_play: 1, lower_priority: 4, total: 10 },
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

function frame(scopes: readonly WorklistScope[], owner = "") {
  globalThis.fetch = (async (): Promise<Response> =>
    jsonResponse(roster)) as typeof fetch;
  return (
    <StoryProviders>
      <WorklistHeader
        day={day(scopes)}
        loaded={8}
        scope="mine"
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
