// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { DealActions } from "./dealactions";

// The verbs a deal's header offers: mail first, a hairline, then what the
// reader RECORDS about the deal, and everything rarer behind the overflow. The
// two things worth looking at are the seam — the row has to read as two groups
// rather than one toolbar — and the refusals, which point every verb at the
// page's one sentence instead of printing it four times.

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const STAGES: Stage[] = [
  {
    id: "st-1",
    pipeline_id: "pl-1",
    name: "Qualified",
    position: 0,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "st-2",
    pipeline_id: "pl-1",
    name: "Proposal",
    position: 1,
    semantic: "open",
    win_probability: 50,
  },
];

const deal = (over: Partial<Deal> = {}): Deal =>
  ({
    id: DEAL_ID,
    name: "Fleet telematics rollout",
    amount_minor: 4_500_000,
    currency: "EUR",
    stage_id: "st-1",
    status: "open",
    stalled: false,
    source: "ui",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as Deal;

const meta: Meta = {
  title: "Records/Deal 360/Actions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const actions = (
  over: Partial<Deal> = {},
  grants: Parameters<typeof meRoute>[0] = {
    deal: ["read", "update"],
    activity: ["create"],
  },
  refusedReasonId?: string,
) => {
  installFetchStub({ "GET /me": meRoute(grants) });
  return (
    <StoryProviders>
      <div className="record-actions">
        <DealActions
          deal={deal(over)}
          openStages={STAGES}
          refusedReasonId={refusedReasonId}
        />
      </div>
      {refusedReasonId && (
        <p id={refusedReasonId}>This deal is not yours to change.</p>
      )}
    </StoryProviders>
  );
};

/** An open deal a rep may write to: every verb live, the seam between groups. */
export const Open: Story = { render: () => actions() };

/**
 * A seat that may read the deal but not log against it. The two recording verbs
 * keep their place and point at one sentence — withholding twelve buttons
 * individually is noise; withholding the explanation is the defect.
 */
export const LoggingRefused: Story = {
  render: () => actions({}, { deal: ["read"] }),
};

/**
 * Archived. The page's own sentence refuses every verb at once, and Reopen is
 * DRAWN (refused) rather than absent, because a reader who archived a closed
 * deal came here asking whether it can come back.
 */
export const Archived: Story = {
  render: () =>
    actions(
      { status: "won", archived_at: "2026-09-01T00:00:00Z" },
      { deal: ["read", "update"], activity: ["create"] },
      "deal-archived",
    ),
};

/**
 * The overflow opened on a closed deal: Reopen is offered, with Archive under
 * it — the fixed order a reader learns once and keeps.
 *
 * Two things about driving it. The panel is PORTALLED to the body, so its rows
 * are reached through `screen` and never through a canvas-scoped query; and
 * `OverflowMenu` defers them to the first open, so they are FOUND after the
 * press rather than got before it.
 */
export const OverflowOnAClosedDeal: Story = {
  render: () => actions({ status: "lost" }),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "More actions",
      }),
    );
    await expect(
      await screen.findByRole("button", { name: "Reopen" }),
    ).toBeVisible();
    await expect(
      await screen.findByRole("button", { name: "Archive deal" }),
    ).toBeVisible();
  },
};
