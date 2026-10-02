import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { BookingBlocked } from "./booking-picker";

import "./book.css";

// The week grid itself renders inside the invite page's stories, in every
// mode; this is the state that stands in for it before a calendar is set up.
const meta: Meta<typeof BookingBlocked> = {
  title: "Patterns/Booking/Time picker",
  component: BookingBlocked,
};
export default meta;
type Story = StoryObj<typeof BookingBlocked>;

export const CalendarBlocked: Story = {
  render: bookingFrame(() => <BookingBlocked />),
};
