import type { Meta, StoryObj } from "@storybook/react-vite";
import { UTC_ZONE } from "../format/timezone";
import { bookingFrame } from "./book.storykit";
import { bookingContact, bookingSlots } from "./book.testkit";
import { BookingProposal } from "./booking-proposal";

import "./book.css";

const meta: Meta<typeof BookingProposal> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingProposal",
  component: BookingProposal,
};
export default meta;
type Story = StoryObj<typeof BookingProposal>;
export const Light: Story = {
  render: bookingFrame(() => (
    <BookingProposal
      request={{
        contact_id: bookingContact.id,
        attendee_email: bookingContact.primary_email,
        subject: "Project discovery",
        description: "Discuss the scope and next steps.",
        location: "Video call",
        duration_minutes: 30,
        options: bookingSlots,
      }}
      zone={UTC_ZONE}
      personalOnly={false}
    />
  )),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };
