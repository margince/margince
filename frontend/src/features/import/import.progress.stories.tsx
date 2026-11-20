// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { ImportProgressPanel } from "./import.progress";
import type { ImportRun } from "./importtypes";

// A run writing records: its phase, the bars, the figures and the counts so
// far, which sit in a strip sized to the numbers.

const meta: Meta = {
  title: "Settings/Data/Data import/Progress",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const importing: ImportRun = {
  id: "019ff-run-running",
  connector: "csv",
  file_name: "hubspot-export.zip",
  source_name: "hubspot-export",
  checkpoint: 880,
  version: 3,
  source_released: false,
  source: "import_api",
  captured_by: "human:u-ada",
  created_at: "2026-10-12T09:29:00Z",
  updated_at: "2026-10-12T09:33:00Z",
  status: "running",
  phase: "importing",
  phase_object: "contact",
  started_at: "2026-10-12T09:30:00Z",
  elapsed_seconds: 205,
  idle_seconds: 2,
  heartbeat_age_seconds: 1,
  rate_rows_per_second: 35,
  eta_seconds_low: 120,
  eta_seconds_high: 240,
  eta_scope: "run",
  progress: {
    objects: [
      { object: "company", rows_done: 140, rows_total: 140 },
      { object: "contact", rows_done: 740, rows_total: 1020 },
      { object: "links", rows_done: 0, rows_total: 1020 },
    ],
    counts: {
      created: 820,
      updated: 40,
      unchanged: 12,
      skipped: 3,
      problems: 1,
    },
  },
};

function panel() {
  return (
    <StoryProviders>
      <ImportProgressPanel run={importing} waiting="Waiting to start" />
    </StoryProviders>
  );
}

/** A run partway through its contacts, with the counts so far. */
export const Importing: Story = { render: panel };

export const ImportingDark: Story = {
  globals: { theme: "dark" },
  render: panel,
};

export const ImportingPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: panel,
};
