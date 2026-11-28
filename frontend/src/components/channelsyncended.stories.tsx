import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { ChannelSyncEnded } from "./channelsyncended";

const meta = {
  title: "Records/Team chat/Ended sync",
  component: ChannelSyncEnded,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ChannelSyncEnded>;
export default meta;
type Story = StoryObj<typeof meta>;
export const HistoryRemains: Story = {
  args: {
    canEdit: true,
    sync: {
      provider: "slack",
      ended_at: "2026-06-02T10:00:00Z",
      read_through: "2026-06-02T09:40:00Z",
      last_posted_at: "2026-06-02T09:30:00Z",
      retained_messages: 2,
      confirmed_redactions: 1,
      unverified_withdrawals: 1,
      unconfirmed_deliveries: 0,
    },
  },
};
export const Reader: Story = {
  ...HistoryRemains,
  args: { ...HistoryRemains.args, canEdit: false },
};
export const HistoryRemainsDark: Story = {
  ...HistoryRemains,
  globals: { theme: "dark" },
};
