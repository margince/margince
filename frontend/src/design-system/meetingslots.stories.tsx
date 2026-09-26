import type { Meta, StoryObj } from "@storybook/react-vite";
import { MeetingSlots } from "./meetingslots";

const meta: Meta<typeof MeetingSlots> = {
  title: "Design System/Meeting slots",
  component: MeetingSlots,
};
export default meta;
type Story = StoryObj<typeof MeetingSlots>;
export const Available: Story = {
  args: {
    slots: [
      {
        start: "2026-10-01T09:00:00Z",
        end: "2026-10-01T09:30:00Z",
        label: "Thursday, 1 October · 11:00",
      },
      {
        start: "2026-10-01T10:00:00Z",
        end: "2026-10-01T10:30:00Z",
        label: "Thursday, 1 October · 12:00",
      },
    ],
    selected: "2026-10-01T09:00:00Z",
    empty: "No times available",
    onSelect: () => undefined,
  },
};
export const Empty: Story = { args: { ...Available.args, slots: [] } };
