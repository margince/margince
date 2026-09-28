import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { BookingCalendars } from "./booking-calendars";

import "./book.css";

const meta: Meta<typeof BookingCalendars> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingCalendars",
  component: BookingCalendars,
};
export default meta;
type Story = StoryObj<typeof BookingCalendars>;
export const Light: Story = {
  render: bookingFrame(() => (
    <BookingCalendars
      provider="gcal"
      calendar="primary"
      blocking={[]}
      onCalendar={() => {}}
      onBlocking={() => {}}
    />
  )),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };
