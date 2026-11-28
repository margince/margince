import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { ChannelProviderCopies } from "./channelprovidercopies";

const meta = {
  title: "Records/Team chat/Provider copies",
  component: ChannelProviderCopies,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ChannelProviderCopies>;
export default meta;
type Story = StoryObj<typeof meta>;
export const RetainedAndUnverified: Story = {
  args: {
    copies: {
      retained_messages: 2,
      confirmed_redactions: 1,
      unverified_withdrawals: 1,
      unconfirmed_deliveries: 1,
    },
  },
};
export const VerifiedRedactions: Story = {
  args: {
    copies: {
      retained_messages: 0,
      confirmed_redactions: 2,
      unverified_withdrawals: 0,
      unconfirmed_deliveries: 0,
    },
  },
};
export const RetainedAndUnverifiedDark: Story = {
  ...RetainedAndUnverified,
  globals: { theme: "dark" },
};
