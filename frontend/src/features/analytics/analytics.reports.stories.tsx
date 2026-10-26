// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportCard } from "./analytics.reports";

const meta: Meta = { title: "Records/Reports/Report card" };
export default meta;

type Story = StoryObj;

function card() {
  installFetchStub({
    "POST /reports/pipeline-current": () =>
      jsonResponse({
        report: "pipeline-current",
        plan: {},
        columns: [],
        rows: [],
        as_of: "2026-03-04T09:00:00Z",
        timezone: "Europe/Berlin",
        base_currency: "EUR",
        fiscal_year_start_month: 1,
      }),
  });
  return (
    <StoryProviders>
      <ReportCard report="pipeline-current" stages={[]} locale="en" />
    </StoryProviders>
  );
}

// The frame names the zone as Intl's long name plus offset, never the IANA id.
export const FinishedRun: Story = {
  render: card,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const frame = await canvas.findByText(/As of/);
    await expect(frame.textContent).toMatch(/UTC/);
    await expect(frame.textContent).not.toContain("Europe/Berlin");
  },
};
