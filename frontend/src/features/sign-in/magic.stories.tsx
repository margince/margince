// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en } from "../../i18n/en";
import type { MagicReceipt } from "../../lib/magic.queries";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { MagicPanel } from "./magic";

const meta: Meta<typeof MagicPanel> = {
  title: "Shell/Home/Receipt",
  component: MagicPanel,
};
export default meta;
type Story = StoryObj<typeof MagicPanel>;

const QUIET: MagicReceipt = {
  as_of: "2026-09-13T08:00:00Z",
  since: "2026-09-12T08:00:00Z",
  done: [],
  needs_you: [],
  could_not_complete: [],
  watching: [],
  totals: { done: 0, needs_you: 0, could_not_complete: 0, watching: 0 },
  not_shown: [],
  sources_unavailable: [],
};

function panel(receipt: MagicReceipt) {
  stubWithSession(
    { "GET /magic": () => jsonResponse(receipt) },
    { deal: ["read", "update"] },
  );
  return (
    <StoryProviders>
      <MagicPanel />
    </StoryProviders>
  );
}

const EVERY_LANE: MagicReceipt = {
  ...QUIET,
  done: [
    {
      id: "00000000-0000-7000-8000-000000000001",
      occurred_at: "2026-09-13T07:30:00Z",
      lane: "done",
      summary: { key: "magic.action.advance_stage" },
      consequence: "magic.consequence.stage_moved",
      entity: {
        type: "deal",
        id: "00000000-0000-7000-8000-0000000000aa",
        label: "Fleet retrofit",
      },
      undo: {
        undoable: true,
        audit_id: "00000000-0000-7000-8000-0000000000bb",
      },
      actor: { type: "agent", id: "runner" },
    },
  ],
  needs_you: [
    {
      id: "00000000-0000-7000-8000-000000000002",
      occurred_at: "2026-09-13T06:10:00Z",
      lane: "needs_you",
      summary: {
        key: "magic.action.approval_send_email",
        values: { target: "Anna Weber" },
      },
      consequence: "magic.consequence.awaits_your_decision",
      actor: { type: "agent", id: "runner" },
    },
  ],
  could_not_complete: [
    {
      id: "00000000-0000-7000-8000-000000000003",
      occurred_at: "2026-09-13T05:02:00Z",
      lane: "could_not_complete",
      summary: {
        key: "magic.action.automation_troubled",
        values: {
          name: "Nightly follow-up",
          outcome: "failed",
          reason_code: "target_missing",
        },
      },
      consequence: "magic.consequence.automation_did_nothing",
      undo: { undoable: false, reason: "no_completed_change" },
      actor: { type: "system", id: "automations" },
    },
  ],
  watching: [
    {
      id: "00000000-0000-7000-8000-000000000004",
      occurred_at: "2026-09-13T04:00:00Z",
      lane: "watching",
      summary: {
        key: "magic.action.capture_reauth_required",
        values: { provider: "Gmail", account: "anna@example.com" },
      },
      consequence: "magic.consequence.capture_not_collecting",
      undo: { undoable: false, reason: "no_completed_change" },
      actor: { type: "connector", id: "gmail" },
    },
  ],
  totals: { done: 1, needs_you: 1, could_not_complete: 1, watching: 1 },
};

export const EveryLane: Story = { render: () => panel(EVERY_LANE) };

/** Nothing ran: the panel stays away rather than drawing four empty lanes. */
export const QuietNight: Story = { render: () => panel(QUIET) };

/** A source that did not load holds the section up by itself, and names
 *  beside it the one the reader may not see. */
export const ASourceDidNotLoad: Story = {
  render: () =>
    panel({
      ...QUIET,
      sources_unavailable: [
        { source: "approval", reason: "failed" },
        { source: "automation_health", reason: "withheld" },
      ],
    }),
};

/** What the reader cannot use is no news: housekeeping, a record type this
 *  build cannot place and a withheld count draw nothing, as a quiet night. */
export const OnlyWhatTheReaderCannotUse: Story = {
  render: () =>
    panel({
      ...QUIET,
      not_shown: [
        { reason: "unadmitted_action", count: 12 },
        { reason: "unknown_entity_type", count: 2 },
      ],
      not_shown_count_withheld: true,
      sources_unavailable: [{ source: "approval", reason: "withheld" }],
    }),
};

/** The read refused: one line and Retry, never a quiet morning. */
export const DidNotLoad: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /magic": () =>
          jsonResponse({ title: "Server Error", status: 500 }, 500),
      },
      { deal: ["read"] },
    );
    return (
      <StoryProviders>
        <MagicPanel />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText(en["magic.failed"]);
    await expect(
      canvas.getByRole("button", { name: en["state.retry"] }),
    ).toBeTruthy();
  },
};

/** Lanes the server read to their bound: the done fold says "at least", and a
 *  cut waiting lane says there is more than it shows. */
export const LanesCutAtTheirBound: Story = {
  render: () =>
    panel({
      ...EVERY_LANE,
      cut_at_limit: ["done", "needs_you", "could_not_complete"],
    }),
  play: async ({ canvasElement }) => {
    const notes = await within(canvasElement).findAllByText(
      en["magic.laneCut"],
    );
    await expect(notes).toHaveLength(2);
  },
};
