import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { NoticeRow } from "./notificationrow";

const meta: Meta<typeof NoticeRow> = {
  title: "Shell/Notification row",
  component: NoticeRow,
  decorators: [
    (Story) => (
      <StoryProviders>
        <ul className="notifbell-list">
          <Story />
        </ul>
      </StoryProviders>
    ),
  ],
  args: {
    notice: {
      id: "notice-1",
      kind: "automation_failed",
      subject: "An automation could not run",
      created_at: "2026-09-15T09:00:00Z",
      target: { type: "deal", id: "22222222-2222-4222-8222-222222222222" },
    },
    onSettle: () => {},
    onActivate: () => {},
    settling: false,
  },
};
export default meta;
type Story = StoryObj<typeof NoticeRow>;
export const Unread: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
export const Settling: Story = { args: { settling: true } };
