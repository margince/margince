// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import type { NoticeCase } from "./noticecases.logic";
import { NoticeSourceReview } from "./noticesource";

const unconfirmed = {
  id: "case-source",
  contact_id: "contact-source",
  contact_name: "Dana Example",
  rule: "unassessed",
  state: "source_review",
  due_at: null,
  attempts: 0,
  created_at: "2026-01-01T10:00:00Z",
  allowed_routes: [],
} satisfies NoticeCase;

const meta: Meta<typeof NoticeSourceReview> = {
  title: "Settings/Compliance/Privacy and retention/Source review",
  component: NoticeSourceReview,
  decorators: [
    (Story) => {
      stubWithSession(
        {
          "GET /privacy/notice-case-reviews/case-source/source": () =>
            jsonResponse({ items: [] }),
          "GET /privacy/notice-case-reviews/case-source": () =>
            jsonResponse({
              ...unconfirmed,
              acquisition: {
                kind: "unknown_legacy",
                captured_at: "2025-11-03T09:00:00Z",
                captured_by: "system",
              },
            }),
        },
        { privacy_request: ["read", "update"] },
      );
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: { duty: unconfirmed },
};
export default meta;
type Story = StoryObj<typeof NoticeSourceReview>;
export const Unconfirmed: Story = {};
export const AssessmentForm: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Review source",
      }),
    );
  },
};

/** "Another source" opens the kinds the first list does not name. */
export const AnotherSource: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Review source",
      }),
    );
    const drawer = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await drawer.findByRole("radio", { name: "Another source" }),
    );
  },
};
