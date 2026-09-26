import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingContact, bookingProfile } from "./book.testkit";
import { BookingInviteScreen } from "./booking-invite";
import { jsonResponse } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingInviteScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingInviteScreen",
  component: BookingInviteScreen,
};
export default meta;
type Story = StoryObj<typeof BookingInviteScreen>;
export const Light: Story = {
  render: bookingFrame(() => (
    <BookingInviteScreen contactId={bookingContact.id} />
  )),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

export const CalendarSetup: Story = {
  render: bookingFrame(
    () => <BookingInviteScreen contactId={bookingContact.id} />,
    {
      "GET /scheduling/profile": () =>
        jsonResponse({ ...bookingProfile, provider: "", enabled: false }),
    },
  ),
};
export const CalendarSetupDark: Story = {
  ...CalendarSetup,
  globals: { theme: "dark" },
};
