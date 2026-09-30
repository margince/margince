import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import {
  bookingConnection,
  bookingContact,
  bookingProfile,
} from "./book.testkit";
import { BookingInviteScreen } from "./booking-invite";
import { jsonResponse } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingInviteScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/Invite page",
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

export const BusyWeek: Story = {
  render: bookingFrame(
    () => <BookingInviteScreen contactId={bookingContact.id} />,
    {
      "GET /availability": () => jsonResponse({ slots: [], truncated: false }),
    },
  ),
};
export const BusyWeekDark: Story = { ...BusyWeek, globals: { theme: "dark" } };

// Outlook names Microsoft Teams in the video switch; the rest of the flow is
// the same whichever calendar sends the invite.
export const OutlookCalendar: Story = {
  render: bookingFrame(
    () => <BookingInviteScreen contactId={bookingContact.id} />,
    {
      "GET /scheduling/profile": () =>
        jsonResponse({ ...bookingProfile, provider: "graphcal" }),
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              ...bookingConnection,
              provider: "graphcal",
              scopes: ["Calendars.ReadWrite"],
            },
          ],
        }),
    },
  ),
};
export const OutlookCalendarDark: Story = {
  ...OutlookCalendar,
  globals: { theme: "dark" },
};
