// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealCockpit } from "./dealcockpit";

// The band a reader scans before reading anything: where this deal stands in
// the pipeline. It used to carry three readings beside the ladder; each now
// lives where the rest of the record already says the same fact (the head's
// facts strip, the brief, the Deal Room tab), so the one story below is the
// ladder itself, refusing to advance a deal that takes no changes.

type Deal = components["schemas"]["Deal"];

const meta: Meta = {
  title: "Records/Deal 360",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const STAGES: components["schemas"]["Stage"][] = [
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

const cockpit = (dealOver: Partial<Deal> = {}, advanceRefused = false) => (
  <DealCockpit
    deal={deal(dealOver)}
    stages={STAGES}
    advancing={false}
    advanceRefused={advanceRefused}
    onAdvance={() => {}}
  />
);

/** An open deal, mid-pipeline, free to advance. */
export const Open: Story = {
  render: () => <StoryProviders>{cockpit()}</StoryProviders>,
};

/** A closed deal: the ladder's own sentence says why it takes no move. */
export const Closed: Story = {
  render: () => (
    <StoryProviders>{cockpit({ status: "won" }, true)}</StoryProviders>
  ),
};
