// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Receipt } from "../../../lib/handled";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealMarginceLine } from "./dealmarginceline";

// The line under a deal's header for a close Margince made: Undo while the
// server allows it, Reopen once it no longer does.

const DEAL_ID = "01a03000-0000-7000-8000-000000000003";

const close = (canUndo: boolean): Receipt => ({
  id: "01a03000-0000-7000-8000-0000000000a1",
  kind: "deal_close_lost",
  summary: "No word from the customer for 196 days",
  occurred_at: "2026-11-20T02:00:00Z",
  subject: { type: "deal", id: DEAL_ID, label: "Fleet telematics rollout" },
  undo: {
    audit_log_id: "01a03000-0000-7000-8000-0000000000a1",
    version: 7,
    reversed: false,
  },
  review: {
    kind: "close_lost",
    accepted: false,
    reversed: false,
    version: 7,
    can_undo: canUndo,
    can_accept: canUndo,
    writable: true,
  },
});

const meta: Meta = {
  title: "Records/Deal 360/Margince line",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const line = (canUndo: boolean) => {
  installFetchStub({ "GET /me": meRoute({ deal: ["read", "update"] }) });
  return (
    <StoryProviders>
      <DealMarginceLine
        dealId={DEAL_ID}
        dealVersion={7}
        receipt={close(canUndo)}
        openStages={[
          {
            id: "01a03000-0000-7000-8000-0000000000e1",
            name: "Discovery",
            semantic: "open",
          } as never,
        ]}
      />
    </StoryProviders>
  );
};

/** The close is still the deal's newest move, so the line offers Undo. */
export const ClosedAndUndoable: Story = {
  render: () => line(true),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", { name: "Undo" }),
    ).toBeVisible();
  },
};

/** The deal changed since the close, so the line offers Reopen instead. */
export const ClosedAndNotUndoable: Story = {
  render: () => line(false),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", { name: "Reopen" }),
    ).toBeVisible();
  },
};
