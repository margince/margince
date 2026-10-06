// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BriefChanges } from "./brief.changes";
import { digest } from "./brief.fixtures";
import { OvernightDigest } from "./brief.rail.overnight";
import { MagicPanel } from "./magic";
import { BUSY_NIGHT } from "./magic.fixtures";
import type { MagicReceipt } from "./magic.queries";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";
import { automaticStageReceipt } from "./worklist.receiptreview.fixtures";

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

/** On Home: the changes still waiting for a word lead, the night's digest closes. */
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
        "GET /digest": () => jsonResponse(digest),
        "GET /projects/01a00000-0000-7000-8000-000000000001": () =>
          jsonResponse({
            id: "01a00000-0000-7000-8000-000000000001",
            name: "ERP replacement",
          }),
        "GET /projects/01a00000-0000-7000-8000-000000000002": () =>
          jsonResponse({
            id: "01a00000-0000-7000-8000-000000000002",
            name: "Depot rollout",
          }),
      },
      { deal: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <MagicPanel lead={<BriefChanges />} foot={<OvernightDigest />} />
      </StoryProviders>
    );
  },
};

/** Nothing ran, and the page may honestly say so. */
export const QuietNight: Story = { render: () => panel(QUIET) };

/** A lane the reader may not see: no lane below it may claim to be clear. */
export const ASourceWithheld: Story = {
  render: () =>
    panel({
      ...QUIET,
      sources_unavailable: [{ source: "approval", reason: "withheld" }],
    }),
};
