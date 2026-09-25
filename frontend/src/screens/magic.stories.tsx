// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MagicPanel } from "./magic";
import type { MagicReceipt } from "./magic.queries";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

const meta: Meta<typeof MagicPanel> = {
  title: "Shell/Home receipt",
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

export const EveryLane: Story = {
  render: () =>
    panel({
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
          undo: { undoable: false, reason: "no_completed_change" },
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
    }),
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
