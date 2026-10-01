import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { bookingFrame } from "./book.storykit";
import { bookingInvitation } from "./book.testkit";
import { BookingMeetingScreen } from "./booking-meeting";
import { DeliveryCard } from "./booking-meeting-parts";
import { jsonResponse } from "./story-utils";

import "./book.css";
import "./booking-meeting.css";

const meta: Meta<typeof BookingMeetingScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/Meeting page",
  component: BookingMeetingScreen,
};
export default meta;
type Story = StoryObj<typeof BookingMeetingScreen>;
export const Light: Story = {
  render: bookingFrame(() => <BookingMeetingScreen token="private-link" />),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

// The host's own page for one invitation, in whatever state the story names.
function hostStory(
  meeting: Partial<components["schemas"]["MeetingInvitation"]>,
): Story {
  return {
    render: bookingFrame(
      () => <BookingMeetingScreen id={bookingInvitation.id} />,
      {
        [`GET /scheduling/invitations/${bookingInvitation.id}`]: () =>
          jsonResponse({ ...bookingInvitation, ...meeting }),
      },
    ),
  };
}

export const ReminderUnavailable: Story = hostStory({
  status: "confirmed",
  reminder_status: "unavailable",
});
export const ReminderUnavailableDark: Story = {
  ...ReminderUnavailable,
  globals: { theme: "dark" },
};

// Accepted by the calendar, with the join link it made and the way into the
// event there; the guest's own reply is still to come.
export const ConfirmedWithVideo: Story = hostStory({
  status: "confirmed",
  provider: "gcal",
  video_call: true,
  video_url: "https://meet.google.com/abc-defg-hij",
  calendar_url: "https://calendar.google.com/calendar/event?eid=abc",
  reminder_status: "pending",
});
export const ConfirmedWithVideoDark: Story = {
  ...ConfirmedWithVideo,
  globals: { theme: "dark" },
};
export const ConfirmedWithVideoPhone: Story = {
  ...ConfirmedWithVideo,
  tags: ["uat-phone"],
};

// Still on its way to the calendar: the link is promised, not shown.
export const Pending: Story = hostStory({
  status: "pending",
  provider: "graphcal",
  video_call: true,
});
export const PendingDark: Story = { ...Pending, globals: { theme: "dark" } };

// The calendar accepted the event without making a link.
export const ConfirmedWithoutLink: Story = hostStory({
  status: "confirmed",
  provider: "gcal",
  video_call: true,
});

// The calendar refused it: retry leads, cancelling comes last.
export const NeedsAttention: Story = hostStory({
  status: "needs_attention",
  provider: "gcal",
  video_call: true,
});
export const NeedsAttentionDark: Story = {
  ...NeedsAttention,
  globals: { theme: "dark" },
};
export const NeedsAttentionPhone: Story = {
  ...NeedsAttention,
  tags: ["uat-phone"],
};

// The host's delivery card through every state an invitation passes, side by
// side, so the tracker's marks can be compared in one look.
export const DeliverySteps: Story = {
  render: bookingFrame(() => (
    <div className="bookmeet">
      {(
        [
          "pending",
          "confirmed",
          "needs_attention",
          "rescheduling",
          "canceling",
          "canceled",
        ] as const
      ).map((status) => (
        <DeliveryCard key={status} status={status} />
      ))}
    </div>
  )),
};
export const DeliveryStepsDark: Story = {
  ...DeliverySteps,
  globals: { theme: "dark" },
};
