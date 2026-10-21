// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { MailSendNotices } from "./compose.parts";

const meta = {
  title: "Patterns/Compose mail/Recipient warning",
  component: MailSendNotices,
  parameters: { layout: "padded" },
  args: {
    to: ["mira@example.com", "alex@example.com"],
    cc: [],
    context: "marketing",
  },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof MailSendNotices>;
export default meta;
type Story = StoryObj<typeof meta>;
export const MultipleRecipients: Story = {};
export const CopiedRecipient: Story = {
  args: { to: ["mira@example.com"], cc: ["alex@example.com"] },
};
