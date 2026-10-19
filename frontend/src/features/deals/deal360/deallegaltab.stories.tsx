// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../../testing/story-utils";
import {
  changesNeeded,
  DEAL_ID,
  legalDeal,
  legalOn,
  summary,
} from "../../privacy/legalreviews.fixtures";
import type { LegalReviewSummary } from "../../privacy/legalreviews.queries";
import { DealLegalTab } from "./deallegaltab";

// A deal's legal review tab: what the deal's side asked and what legal said.
const meta: Meta = { title: "Records/Deal 360/Legal review tab" };
export default meta;

type Story = StoryObj;

function tab(reviews: LegalReviewSummary[]) {
  stubWithSession(
    {
      "GET /legal-reviews/settings": legalOn,
      [`GET /deals/${DEAL_ID}/legal-reviews`]: () =>
        jsonResponse({ data: reviews }),
    },
    { deal: ["read", "update"], legal_review: ["read"] },
  );
  return (
    <StoryProviders>
      <DealLegalTab deal={legalDeal} />
    </StoryProviders>
  );
}

/** No review asked for yet: the request is the one verb. */
export const NoReview: Story = { render: () => tab([]) };

/** A request waits for legal: it can be withdrawn, not sent twice. */
export const Waiting: Story = { render: () => tab([summary()]) };

/** Legal asked for changes: the comments, and the verb to ask again. */
export const ChangesNeeded: Story = { render: () => tab([changesNeeded]) };

/** An approval after changes, with the earlier answer beneath it. */
export const Approved: Story = {
  render: () =>
    tab([
      summary({
        status: "approved",
        decided_by_name: "Lena Legal",
        decided_at: "2026-10-02T09:00:00Z",
        comments: "Fine as revised.",
      }),
      changesNeeded,
    ]),
};
