// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { MagicReceipt } from "../../lib/magic.queries";
import { BUSY_NIGHT } from "../../testing/magic.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { automaticStageReceipt } from "../../testing/worklist.receiptreview.fixtures";
import { BriefChanges } from "../brief/brief.changes";
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
  stubWithSession({ "GET /magic": () => jsonResponse(receipt) }, {});
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
        values: { name: "Nightly follow-up", outcome: "timed out" },
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
  not_shown: [{ reason: "out_of_scope", count: 12 }],
};

export const EveryLane: Story = { render: () => panel(EVERY_LANE) };

/** On Home: the receipt in the work column, the changes waiting for a word in the rail. */
export const OnHome: Story = {
  render: () => {
    // Accept and Undo answer as the server would: the change reads as
    // answered on the next read, rather than offering both again.
    const change = automaticStageReceipt;
    const deal = change.subject?.id;
    let review = change.review;
    const answer = (patch: { accepted?: boolean; reversed?: boolean }) => {
      review = review && { ...review, ...patch };
      return new Response(null, { status: 204 });
    };
    stubWithSession(
      {
        "GET /magic": () => jsonResponse(BUSY_NIGHT),
        "GET /worklist/handled": () =>
          jsonResponse({
            as_of: "2026-09-13T08:00:00Z",
            receipts: [{ ...change, review }],
            truncated: false,
          }),
        [`POST /deals/${deal}/applied-changes/${change.id}/accept`]: () =>
          answer({ accepted: true }),
        [`POST /deals/${deal}/stage-progressions/${change.id}/revert`]: () =>
          answer({ reversed: true }),
        [`GET /deals/${deal}`]: () => jsonResponse({ name: "PIM Rollout" }),
      },
      { deal: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <MagicPanel />
        <BriefChanges />
      </StoryProviders>
    );
  },
};

/** Nothing ran: the panel stays away rather than drawing four empty lanes. */
export const QuietNight: Story = { render: () => panel(QUIET) };

/** A lane the reader may not see: no lane below it may claim to be clear. */
export const ASourceWithheld: Story = {
  render: () =>
    panel({
      ...QUIET,
      sources_unavailable: [{ source: "approval", reason: "withheld" }],
    }),
};

export const WithheldCoverage: Story = {
  render: () =>
    panel({ ...EVERY_LANE, not_shown: [], not_shown_count_withheld: true }),
};

export const UnknownReceiptKind: Story = {
  render: () =>
    panel({
      ...QUIET,
      not_shown: [{ reason: "unknown_entity_type", count: 2 }],
    }),
};
