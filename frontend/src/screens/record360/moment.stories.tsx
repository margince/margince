// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { MomentRow } from "./moment";
import { TodayPanel } from "./today";
// The lead row's display face and the interval under its verb live in the
// glance's own sheet, which `company360.css` (imported by the row) does not
// carry — without it the headline is set in the body face, which is the one
// thing this row is drawn differently from every other row FOR.
import "../company/glance.css";

// THE MOMENT as the lead row of the needs list.
//
// Three things are decided here and nothing else: the word for the rule that
// fired, the tone that word carries, and whether the recommended action is a
// button at all. One frame each, because a rule read in the wrong tone and a
// verb that lands nowhere are both invisible to an assertion and obvious in a
// glance. Drawn inside `TodayPanel`, the pane every record page mounts it in:
// the row is a `PanelRow` with no box of its own, so on a bare canvas it is a
// stack of spans with no ground, no inset and no rule above it.

type ContactMoment = components["schemas"]["ContactMoment"];

function moment(over: Partial<ContactMoment> = {}): ContactMoment {
  return {
    claim_key: "moment:overdue_promise",
    evidence_fingerprint: "f-1",
    rule: "overdue_promise",
    headline: "Send Lena the line-item 3 breakdown you promised on 5 August.",
    why_now: "The promise was made 19 days ago and nothing has gone out since.",
    confidence: "observed_fact",
    evidence: [
      {
        type: "activity",
        id: "01a05500-0000-7000-8000-0000000000e1",
        label: "Re: retrofit pricing",
        snippet:
          "I'll get you the line-item 3 breakdown by the end of the week.",
        observed_at: "2026-08-05T09:12:00Z",
      },
      { type: "task", label: "Send the promised breakdown" },
    ],
    recommended_action: {
      kind: "draft_reply",
      label: "Draft it",
      state: "available",
      destination: {
        surface: "composer",
        entity_type: "contact",
        entity_id: "01a05500-0000-7000-8000-0000000000aa",
      },
    },
    ...over,
  };
}

const meta: Meta<typeof MomentRow> = {
  title: "Records/Company 360/The moment",
  component: MomentRow,
  parameters: { layout: "padded" },
  args: { onOpenRecord: fn() },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <TodayPanel>
            <Story />
          </TodayPanel>
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof MomentRow>;

/**
 * SOMEBODY IS BEING KEPT WAITING, the one reading drawn as a warning: the rule
 * word over the headline in the warn dim, the evidence one disclosure away
 * quoting what was actually written rather than naming the kind of record it
 * came from, and the page's one filled verb — indigo, because pressing it hands
 * the writing to Margince.
 */
export const APromiseIsLate: Story = { args: { moment: moment() } };

/**
 * A LIVE THREAD, and a verb the server did not authorise. "Nothing scheduled" is
 * a fact about the relationship rather than a verdict on it, so the dim is the
 * accent; and the action is `blocked`, so there is no button at all — a card
 * whose verb lands nowhere is worse than one with no verb, because the reader
 * presses it, nothing happens, and they stop trusting the ones that work.
 */
export const NothingScheduledAndNothingToPress: Story = {
  args: {
    moment: moment({
      claim_key: "moment:missing_next_step",
      rule: "missing_next_step",
      headline: "Nothing is booked with Brandt after the retrofit review.",
      why_now: "The last meeting closed without a next date being agreed.",
      confidence: "high",
      recommended_action: {
        kind: "schedule_meeting",
        label: "Book the next one",
        state: "blocked",
        blocked_reason: "No calendar is connected for this seat.",
      },
    }),
  },
};

/**
 * THE QUIET ONE, a row only when there is nothing else in the list. It reads as
 * settled rather than as something nobody has judged, so its dim is the calm one
 * — and a list of exactly this is the only arrangement it is drawn in at all.
 */
export const NothingNeededToday: Story = {
  args: {
    moment: moment({
      claim_key: "moment:nothing_needed",
      rule: "nothing_needed",
      headline: "Nothing needs you on this account today.",
      why_now: "Every promise is answered and the next meeting is booked.",
      confidence: "observed_fact",
      evidence: [{ type: "relationship_change", label: "Review held 18 Aug" }],
      recommended_action: {
        kind: "open_record",
        label: "Open",
        state: "available",
      },
    }),
  },
};

/**
 * The late moment in dark, where the warn dim is the whole signal. The rule word
 * carries no ground of its own — an ink over the pane — and the verb beside it
 * is the accent's dark lift. Both are `color-mix()` steps off a ground that
 * inverts, so this frame is what says whether the word still reads as a warning
 * rather than as another caption.
 */
export const APromiseIsLateDark: Story = {
  args: { moment: moment() },
  globals: { theme: "dark" },
};
