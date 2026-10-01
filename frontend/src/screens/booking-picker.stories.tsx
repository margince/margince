import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { BookingBlocked, BookingLinkSummary } from "./booking-picker";

import "./book.css";

// The week grid itself renders inside the invite page's stories; these are the
// two states that stand in for it.
const meta: Meta<typeof BookingLinkSummary> = {
  title: "Patterns/Booking/Time picker",
  component: BookingLinkSummary,
};
export default meta;
type Story = StoryObj<typeof BookingLinkSummary>;

export const GuestPicks: Story = {
  render: bookingFrame(() => <BookingLinkSummary name="Nina Weber" />),
};
export const GuestPicksDark: Story = {
  ...GuestPicks,
  globals: { theme: "dark" },
};
export const CalendarBlocked: Story = {
  render: bookingFrame(() => <BookingBlocked />),
};
