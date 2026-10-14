// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { LegalReviewsRoute } from "./legalreviews";
import {
  legalOn,
  queueItem,
  REVIEW_ID,
  review,
  VIEWER_ID,
} from "./legalreviews.fixtures";
import { LegalReviewSettingsCard } from "./legalreviewsettings";

// The reviewer's side: the queue, one review's pack and answer, and the
// installation switch.
const meta: Meta = { title: "Records/Legal reviews" };
export default meta;

type Story = StoryObj;

const REVIEWER = { legal_review: ["read", "update"] } as const;

/** Largest value first, then the nearest close; no value sorts last. */
export const Queue: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /legal-reviews/settings": legalOn,
        "GET /legal-reviews": () =>
          jsonResponse({
            data: [
              queueItem("Warehouse rollout", 9_000_000),
              queueItem("Fleet renewal", 1_200_000),
              queueItem("Pilot", null),
            ],
            page: { has_more: false },
          }),
      },
      REVIEWER,
    );
    return (
      <StoryProviders>
        <LegalReviewsRoute />
      </StoryProviders>
    );
  },
};

function one(requestedBy?: string) {
  stubWithSession(
    {
      "GET /legal-reviews/settings": legalOn,
      [`GET /legal-reviews/${REVIEW_ID}`]: () =>
        jsonResponse(review(requestedBy ? { requested_by: requestedBy } : {})),
    },
    REVIEWER,
  );
  return (
    <StoryProviders>
      <LegalReviewsRoute id={REVIEW_ID} />
    </StoryProviders>
  );
}

/** The pack with the answer form above it. */
export const Review: Story = { render: () => one() };

/** The requester opening their own review: a colleague answers it. */
export const OwnRequest: Story = { render: () => one(VIEWER_ID) };

/** Settings → Pipelines: the installation's switch. */
export const Switch: Story = {
  render: () => {
    stubWithSession(
      { "GET /legal-reviews/settings": legalOn },
      { installation_settings: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <LegalReviewSettingsCard />
      </StoryProviders>
    );
  },
};
