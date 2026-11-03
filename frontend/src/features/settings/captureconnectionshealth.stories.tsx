import type { Meta, StoryObj } from "@storybook/react-vite";
import { viewerZone } from "../../format/timezone";
import { StoryProviders } from "../../testing/story-utils";
import { CaptureConnectionsHealth } from "./captureconnectionshealth";

const meta = {
  title: "Settings/Governance/System health/Connection concerns",
  component: CaptureConnectionsHealth,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof CaptureConnectionsHealth>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Stalled: Story = {
  args: {
    zone: viewerZone(),
    rows: [
      {
        user_id: "0190b7a2-0000-7000-8000-000000000001",
        connection_id: "0190b7a2-0000-7000-8000-000000000002",
        display_name: "Demo Member",
        provider: "gmail",
        account_label: "mailbox@example.test",
        concern: "sync_stalled",
        since: "2026-10-01T09:00:00Z",
        last_success_at: "2026-10-01T09:00:00Z",
      },
    ],
  },
};
export const Empty: Story = { args: { zone: viewerZone(), rows: [] } };

export const StalledDark: Story = { ...Stalled, globals: { theme: "dark" } };
