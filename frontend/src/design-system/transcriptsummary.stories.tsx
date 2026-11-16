// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { TranscriptReadCard } from "./transcriptread";
import { TranscriptSummary } from "./transcriptsummary";

const LATEST = "GET /activities/a-1/transcript-proposals/latest";
const READ = "GET /activities/a-1/transcript-proposals/rd-1";

const base = {
  read_id: "rd-1",
  activity_id: "a-1",
  status: "done",
  line_count: 48,
  proposal_ids: [],
  proposals: [],
  created_at: "2026-08-01T09:00:00Z",
  summary_refused: false,
};

function stubbed(report: Record<string, unknown>) {
  installFetchStub({
    [LATEST]: () => jsonResponse(report),
    [READ]: () => jsonResponse(report),
    "GET /approvals/ap-1": () =>
      jsonResponse({ id: "ap-1", status: "pending" }),
  });
}

const meta: Meta<typeof TranscriptSummary> = {
  title: "Records/Record 360/Transcript summary",
  component: TranscriptSummary,
};
export default meta;
type Story = StoryObj<typeof TranscriptSummary>;

// The summary a reading wrote, as the History shows it above the transcript.
// It has the AI mark, the key points and the next steps, each with its lines.
const summary = {
  read_id: "rd-1",
  generated_at: "2026-08-01T09:01:00Z",
  key_points: [
    {
      text: "Pallet volumes rose about fifteen percent, which changes the price tiers.",
      source_lines: [3, 4],
    },
    {
      text: "The integration has run without trouble since the upgrade.",
      source_lines: [7],
    },
  ],
  next_steps: [
    { text: "Lars sends the revised pricing by Friday.", source_lines: [4] },
  ],
};

export const Summary: Story = {
  render: () => {
    stubbed({ ...base, proposal_ids: ["ap-1"], summary });
    return (
      <StoryProviders>
        <TranscriptSummary activityId="a-1" />
        <TranscriptReadCard activityId="a-1" />
      </StoryProviders>
    );
  },
};

export const SummaryDark: Story = {
  ...Summary,
  globals: { theme: "dark" },
};

// A later reading whose summary failed its check: the earlier summary stays,
// and the reading box says this reading wrote none.
export const SummaryRefused: Story = {
  render: () => {
    stubbed({
      ...base,
      status_detail: "Nobody committed to anything in this call.",
      summary_refused: true,
      summary,
    });
    return (
      <StoryProviders>
        <TranscriptSummary activityId="a-1" />
        <TranscriptReadCard activityId="a-1" />
      </StoryProviders>
    );
  },
};
