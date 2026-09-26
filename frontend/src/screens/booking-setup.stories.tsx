import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingConnection, bookingProfile } from "./book.testkit";
import { BookingSetup } from "./booking-setup";
import { jsonResponse } from "./story-utils";
import "./book.css";

const meta: Meta<typeof BookingSetup> = {
  title: "Patterns/Booking/Calendar setup",
  component: BookingSetup,
};
export default meta;
type Story = StoryObj<typeof BookingSetup>;
const profile = { ...bookingProfile, provider: "", enabled: false };
export const Connected: Story = {
  render: bookingFrame(() => (
    <BookingSetup profile={{ ...profile, provider: "" }} />
  )),
};
export const ConnectedDark: Story = {
  ...Connected,
  globals: { theme: "dark" },
};
export const ReadOnly: Story = {
  render: bookingFrame(
    () => <BookingSetup profile={{ ...profile, provider: "" }} />,
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              ...bookingConnection,
              scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
            },
          ],
        }),
    },
  ),
};
export const ReadOnlyDark: Story = { ...ReadOnly, globals: { theme: "dark" } };
export const Disconnected: Story = {
  render: bookingFrame(
    () => <BookingSetup profile={{ ...profile, provider: "gcal" }} />,
    { "GET /connectors": () => jsonResponse({ data: [] }) },
  ),
};
