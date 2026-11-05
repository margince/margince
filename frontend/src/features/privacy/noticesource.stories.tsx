// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { NoticeSourceReview } from "./noticesource";

const meta: Meta<typeof NoticeSourceReview> = {
  title: "Settings/Governance/Privacy and retention/Source review",
  component: NoticeSourceReview,
  decorators: [
    (Story) => {
      stubWithSession({}, { privacy_request: ["read", "update"] });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    duty: {
      id: "case-source",
      contact_id: "contact-source",
      contact_name: "Dana Example",
      rule: "unassessed",
      state: "source_review",
      due_at: null,
      attempts: 0,
      created_at: "2026-01-01T10:00:00Z",
      allowed_routes: [],
    },
  },
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
