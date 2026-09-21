// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { RunView } from "./backfillrunview";
import { StoryProviders } from "./story-utils";

// What the import card says about a run in each state the server can hand it.
// The states are kept side by side because three of them look alike at a
// glance and mean different things to whoever is waiting: reading, not moving,
// and stopped. Only the first may draw a bar.
//
// Every figure here is a persisted count and every prop is passed straight in
// — the card reads nothing of its own, so these stories need no routes.

type BackfillStatus = components["schemas"]["BackfillStatus"];

const COUNTS: BackfillStatus["counts"] = {
  messages_scanned: 4120,
  captured: 2874,
  skipped: 1246,
  contacts_created: 318,
  companies_created: 96,
};

function view(run: BackfillStatus, cancelError: string | null = null) {
  return () => (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <RunView
          run={run}
          cancelling={false}
          cancelError={cancelError}
          onCancel={() => {}}
          onRestart={() => {}}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof RunView> = {
  title: "Settings/You/Connections/Backfill run",
  component: RunView,
};
export default meta;

type Story = StoryObj<typeof RunView>;

// The machine is reading: indigo ground, the badge that says so in words, and
// the bar its denominator earns. `updated_at` is deliberately absent — a run
// that has not reported a stamp is not a stale one, and leaving it out keeps
// this story's verdict off whatever day the catalog is opened on.
export const Reading: Story = {
  render: view({
    state: "running",
    window: "12m",
    estimated_messages: 9000,
    counts: COUNTS,
    started_at: "2026-08-15T08:00:00Z",
  }),
};

// A queued run has started nothing yet, so it wears plain ground: indigo is a
// claim about who is doing the work, and nobody is doing any.
export const Queued: Story = {
  render: view({ state: "queued", window: "6m", counts: {} }),
};

// A provider that counts by paging answers a FLOOR, and this run has passed
// it. No bar — a fraction over a denominator the run has already exceeded
// would draw progress past its own end — so the scanned count stands alone.
export const EstimateAlreadyPassed: Story = {
  render: view({
    state: "running",
    window: "24m",
    estimated_messages: 2000,
    estimate_is_floor: true,
    counts: COUNTS,
  }),
};

// A live run whose stamp stopped moving. The bar is gone and a note says how
// long it has been still: a killed worker leaves the row saying "running", and
// drawing progress for it would report work nobody is doing.
export const NotMoving: Story = {
  render: view({
    state: "running",
    window: "12m",
    estimated_messages: 9000,
    counts: COUNTS,
    updated_at: "2026-08-15T08:11:00Z",
  }),
};

// Finished. The figures stop climbing because the server has nothing left to
// count towards, and the verb offered is the window picker again.
export const Done: Story = {
  render: view({
    state: "done",
    window: "12m",
    estimated_messages: 4120,
    counts: COUNTS,
    completed_at: "2026-08-15T09:30:00Z",
  }),
};

// The run failed. The class is all this surface may show — the detail is in
// the system log, and a provider's own text is not ours to render.
export const Failed: Story = {
  render: view({
    state: "error",
    window: "12m",
    counts: COUNTS,
    last_error_class: "provider_rate_limited",
  }),
};

// Somebody pressed stop and the server refused it. The refusal is only ever
// drawn over a run that is still live — once the row has stopped there is
// nothing left for the sentence to be about.
export const StopRefused: Story = {
  render: view(
    {
      state: "running",
      window: "12m",
      estimated_messages: 9000,
      counts: COUNTS,
    },
    "The import had already finished when the stop arrived.",
  ),
};

// Stopped by hand. The note under the figures says the counts are what the run
// reached, not what the window held.
export const Cancelled: Story = {
  render: view({ state: "cancelled", window: "6m", counts: COUNTS }),
};
