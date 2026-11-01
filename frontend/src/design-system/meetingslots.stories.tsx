import type { Meta, StoryObj } from "@storybook/react-vite";
import { type MeetingDay, MeetingSlots, MeetingWeek } from "./meetingslots";

const meta: Meta<typeof MeetingSlots> = {
  title: "Components/Forms and input/Meeting slots",
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
export const AvailableDark: Story = {
  ...Available,
  globals: { theme: "dark" },
};

const WEEK_DAYS: MeetingDay[] = [
  {
    key: "2026-10-05",
    weekday: "Mon",
    date: "5 Oct",
    slots: [
      {
        start: "2026-10-05T07:00:00Z",
        end: "2026-10-05T07:30:00Z",
        label: "Monday, 5 October · 09:00",
        time: "09:00",
      },
      {
        start: "2026-10-05T08:30:00Z",
        end: "2026-10-05T09:00:00Z",
        label: "Monday, 5 October · 10:30",
        time: "10:30",
      },
    ],
  },
  { key: "2026-10-06", weekday: "Tue", date: "6 Oct", slots: [] },
  {
    key: "2026-10-07",
    weekday: "Wed",
    date: "7 Oct",
    slots: [
      {
        start: "2026-10-07T12:00:00Z",
        end: "2026-10-07T12:30:00Z",
        label: "Wednesday, 7 October · 14:00",
        time: "14:00",
      },
    ],
  },
];

export const Week: StoryObj<typeof MeetingWeek> = {
  render: () => (
    <MeetingWeek
      days={WEEK_DAYS}
      selected={["2026-10-05T08:30:00Z"]}
      emptyDay="No free time"
      onSelect={() => undefined}
    />
  ),
};
export const WeekDark: StoryObj<typeof MeetingWeek> = {
  ...Week,
  globals: { theme: "dark" },
};
// A 30-minute choice at 09:00 blocks 09:15, which shares its minutes. 09:30
// starts where it ends, so it can still be pressed.
const mondaySlot = (start: string, end: string, time: string) => ({
  start: `2026-10-05T${start}:00Z`,
  end: `2026-10-05T${end}:00Z`,
  label: `Monday, 5 October · ${time}`,
  time,
});
const BLOCKED_DAY: MeetingDay[] = [
  {
    key: "2026-10-05",
    weekday: "Mon",
    date: "5 Oct",
    slots: [
      mondaySlot("07:00", "07:30", "09:00"),
      mondaySlot("07:15", "07:45", "09:15"),
      mondaySlot("07:30", "08:00", "09:30"),
    ],
  },
];

export const WeekBlocked: StoryObj<typeof MeetingWeek> = {
  render: () => (
    <MeetingWeek
      days={BLOCKED_DAY}
      selected={["2026-10-05T07:00:00Z"]}
      blocked={["2026-10-05T07:15:00Z"]}
      emptyDay="No free time"
      onSelect={() => undefined}
    />
  ),
};
export const WeekBlockedDark: StoryObj<typeof MeetingWeek> = {
  ...WeekBlocked,
  globals: { theme: "dark" },
};
// Without `onSelect`: the times a guest will choose from, shown to the host
// who sends them the link.
export const WeekPreview: StoryObj<typeof MeetingWeek> = {
  render: () => (
    <MeetingWeek days={WEEK_DAYS} selected={[]} emptyDay="No free time" />
  ),
};
export const WeekPreviewDark: StoryObj<typeof MeetingWeek> = {
  ...WeekPreview,
  globals: { theme: "dark" },
};
