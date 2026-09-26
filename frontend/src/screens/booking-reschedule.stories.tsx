import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingSlots } from "./book.testkit";
import { BookingReschedule } from "./booking-reschedule";
import { jsonResponse } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingReschedule> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingReschedule",
  component: BookingReschedule,
};
export default meta;
type Story = StoryObj<typeof BookingReschedule>;
export const Light: Story = {
  render: bookingFrame(
    () => (
      <BookingReschedule
        token="private-link"
        pending={false}
        onSelect={() => {}}
      />
    ),
    {
      "GET /public/meeting/private-link/availability": () =>
        jsonResponse({ slots: bookingSlots, truncated: false }),
    },
  ),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };
