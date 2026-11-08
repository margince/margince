// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactSourceReview } from "./contactsourcereview";

const review = {
  id: "case-source",
  contact_id: "contact-source",
  contact_name: "Dana Example",
  rule: "unassessed",
  state: "source_review",
  due_at: null,
  attempts: 0,
  created_at: "2026-01-01T10:00:00Z",
  allowed_routes: [],
  acquisition: {
    kind: "unknown_legacy",
    captured_at: "2025-11-03T09:00:00Z",
    captured_by: "system",
  },
};

function officer(grants: ("read" | "update")[]) {
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source/source": () =>
        jsonResponse({ items: [] }),
      "GET /privacy/notice-case-reviews/case-source": () =>
        jsonResponse(review),
    },
    { privacy_request: grants },
  );
}

const meta: Meta<typeof ContactSourceReview> = {
  title: "Records/Contact 360/Source review",
  component: ContactSourceReview,
  decorators: [
    (Story) => {
      officer(["read", "update"]);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: { reviews: ["case-source"] },
};
export default meta;
type Story = StoryObj<typeof ContactSourceReview>;
export const SourceReview: Story = {};

export const MultipleSourceReviews: Story = {
  args: { reviews: ["case-source", "case-source-2"] },
};

/** A reader who may not assess the source is told who does. */
export const ReadOnly: Story = {
  decorators: [
    (Story) => {
      officer(["read"]);
      return <Story />;
    },
  ],
};

/** The drawer over the record: the reason, what is on record, the question. */
export const Drawer: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Review source",
      }),
    );
  },
};
