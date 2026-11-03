// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  type DecisionReview,
  DecisionsPanel,
  type SalesDecision,
} from "./analytics.decisions";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

// Decisions from the last review. On the Sales review the panel lists the
// decisions on the deals of the review's selection.

const meta: Meta = {
  title: "Records/Reports/Analytics/Review decisions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const decisions: SalesDecision[] = [
  {
    id: "11111111-1111-4111-8111-111111111111",
    decision: "Call the buyer about the budget",
    deal_id: "22222222-2222-4222-8222-222222222222",
    owner_id: "00000000-0000-4000-8000-000000000001",
    review_on: "2026-11-10",
    expected_result: "Budget owner named",
    status: "open",
    done_at: null,
    result: null,
    created_at: "2026-11-03T09:00:00Z",
  },
  {
    id: "33333333-3333-4333-8333-333333333333",
    decision: "Send the revised fleet offer",
    deal_id: "44444444-4444-4444-8444-444444444444",
    owner_id: "00000000-0000-4000-8000-000000000001",
    review_on: "2026-10-27",
    expected_result: "Offer signed",
    status: "done",
    done_at: "2026-10-29T15:00:00Z",
    result: "Offer sent, signature due Friday",
    created_at: "2026-10-20T09:00:00Z",
  },
];

function panel(review?: DecisionReview) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": meRoute({
      report_definition: ["read"],
      deal: ["read"],
      activity: ["read", "create", "update"],
    }),
    "GET /analytics/decisions": () => jsonResponse({ data: decisions }),
  });
  return (
    <StoryProviders>
      <DecisionsPanel review={review} />
    </StoryProviders>
  );
}

/** Every decision the reader may see. */
export const AllDecisions: Story = { render: () => panel() };

/** A Sales review narrowed to one team and one country. */
export const ForAReview: Story = {
  render: () =>
    panel({
      scope: { kind: "team", id: "55555555-5555-4555-8555-555555555555" },
      country: "DE",
    }),
};
