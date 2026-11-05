// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { ContactSourceReview } from "./contactsourcereview";

const meta: Meta<typeof ContactSourceReview> = {
  title: "Records/Contact 360/Source review",
  component: ContactSourceReview,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    view: {
      as_of: "2026-01-01T10:00:00Z",
      contact: {
        id: "contact-source",
        full_name: "Dana Example",
        source: "capture",
        captured_by: "connector:gmail",
        created_at: "2026-01-01T10:00:00Z",
        updated_at: "2026-01-01T10:00:00Z",
      },
      sections_omitted: [],
      privacy_source_reviews: ["case-source"],
    },
    onAction: () => undefined,
  },
};
export default meta;
type Story = StoryObj<typeof ContactSourceReview>;
export const SourceReview: Story = {};

export const MultipleSourceReviews: Story = {
  render: (args) => (
    <ContactSourceReview
      {...args}
      view={{
        ...args.view,
        privacy_source_reviews: ["case-source", "case-source-2"],
      }}
    />
  ),
};
