import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingInvitation } from "./book.testkit";
import { BookingMeetingScreen } from "./booking-meeting";
import { jsonResponse } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingMeetingScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingMeetingScreen",
  component: BookingMeetingScreen,
};
export default meta;
type Story = StoryObj<typeof BookingMeetingScreen>;
export const Light: Story = {
  render: bookingFrame(() => <BookingMeetingScreen token="private-link" />),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

export const ReminderUnavailable: Story = {
  render: bookingFrame(
    () => <BookingMeetingScreen id={bookingInvitation.id} />,
    {
      [`GET /scheduling/invitations/${bookingInvitation.id}`]: () =>
        jsonResponse({
          ...bookingInvitation,
          status: "confirmed",
          reminder_status: "unavailable",
        }),
    },
  ),
};
export const ReminderUnavailableDark: Story = {
  ...ReminderUnavailable,
  globals: { theme: "dark" },
};
