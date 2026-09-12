// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { OutcomeReviewPanel } from "./outcomereview";

// The three states this panel has to keep apart, and only one of them is the
// ordinary case. A closed deal with no review yet is an invitation; a review of
// the CURRENT closing is the record; a review of an EARLIER closing is history
// that must not read as a verdict on today's outcome.

const CLOSING = "11111111-1111-1111-1111-111111111111";
const EARLIER = "22222222-2222-2222-2222-222222222222";

const questions = [
  { key: "why", label: "Why did we win?", type: "text", required: true },
  {
    key: "else",
    label: "Who else were they considering?",
    type: "text",
    required: false,
  },
];

const template = {
  id: "t-1",
  key: "win_review",
  label: "Win review",
  outcome: "won",
  questions,
  version: 1,
  active: true,
  system: true,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const review = (over: Record<string, unknown>) => ({
  id: "r-1",
  activity_id: "a-1",
  deal_id: "d-1",
  closing_occurrence_id: CLOSING,
  outcome: "won",
  template_key: "win_review",
  template_version: 1,
  questions,
  answers: { why: "Price and the migration plan." },
  revision: 1,
  created_at: "2026-09-12T10:00:00Z",
  updated_at: "2026-09-12T10:00:00Z",
  ...over,
});

// The grant is what decides whether a reader may write a review — the panel
// asks `activity:create`, the same permission the server checks.
function backend(reviews: unknown[], canWrite = true) {
  installFetchStub({
    "GET /me": meRoute({ activity: canWrite ? ["read", "create"] : ["read"] }),
    "GET /deals/d-1/outcome-reviews": () => jsonResponse({ data: reviews }),
    "GET /activity-review-templates": () => jsonResponse({ data: [template] }),
  });
}

const meta: Meta = {
  title: "Records/Deal 360/Outcome review",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const NoReviewYet: Story = {
  render: () => {
    backend([]);
    return (
      <StoryProviders>
        <OutcomeReviewPanel
          dealId="d-1"
          status="won"
          closingOccurrenceId={CLOSING}
        />
      </StoryProviders>
    );
  },
};

// A reader without write access sees the record and no way in. Absent rather
// than disabled: a disabled button invites a hunt for the reason.
export const ReadOnly: Story = {
  render: () => {
    backend([], false);
    return (
      <StoryProviders>
        <OutcomeReviewPanel
          dealId="d-1"
          status="won"
          closingOccurrenceId={CLOSING}
        />
      </StoryProviders>
    );
  },
};

export const Written: Story = {
  render: () => {
    backend([review({})]);
    return (
      <StoryProviders>
        <OutcomeReviewPanel
          dealId="d-1"
          status="won"
          closingOccurrenceId={CLOSING}
        />
      </StoryProviders>
    );
  },
};

// Reopened and reclosed. The old review stays readable and says which closing
// it was about, so March's loss review is never read as a verdict on June's win.
export const AboutAnEarlierClosing: Story = {
  render: () => {
    backend([
      review({ id: "r-old", closing_occurrence_id: EARLIER, outcome: "lost" }),
    ]);
    return (
      <StoryProviders>
        <OutcomeReviewPanel
          dealId="d-1"
          status="won"
          closingOccurrenceId={CLOSING}
        />
      </StoryProviders>
    );
  },
};
