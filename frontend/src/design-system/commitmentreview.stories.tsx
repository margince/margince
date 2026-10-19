// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { viewerZone } from "../format/timezone";
import { StoryProviders } from "../testing/story-utils";
import { CommitmentReview } from "./commitmentreview";

const meta: Meta<typeof CommitmentReview> = {
  title: "Components/AI and provenance/Commitment review",
  component: CommitmentReview,
  play: async ({ canvasElement, args }) => {
    if (args.decisions?.some((decision) => !decision.current_work)) {
      await userEvent.click(
        within(canvasElement).getByText("Commitment review"),
      );
    }
  },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof CommitmentReview>;
const historical = {
  locator: "pricing",
  summary: "Send revised pricing",
  source_quote: "I will send revised pricing.",
  outcome: "historical_without_current_deadline",
  current_work: false,
  rule_version: "1",
  evidence_ids: [],
  evaluated_at: "2026-10-18T10:00:00Z",
};
export const Historical: Story = {
  args: { zone: viewerZone(), decisions: [historical] },
};
export const NeedsEvidence: Story = {
  args: {
    zone: viewerZone(),
    decisions: [
      { ...historical, outcome: "later_context_not_read" },
      {
        ...historical,
        locator: "dates",
        summary: "Confirm delivery date",
        outcome: "source_time_contradicted",
      },
    ],
  },
};
export const NoWithheldCommitments: Story = {
  args: {
    zone: viewerZone(),
    decisions: [{ ...historical, current_work: true }],
  },
};

export const HistoricalDark: Story = {
  ...Historical,
  globals: { theme: "dark" },
};

export const NeedsEvidenceDark: Story = {
  ...NeedsEvidence,
  globals: { theme: "dark" },
};
