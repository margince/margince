import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingProfile } from "./book.testkit";
import { BookingGuestScreen } from "./booking-guest";
import { jsonResponse } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingGuestScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingGuestScreen",
  component: BookingGuestScreen,
};
export default meta;
type Story = StoryObj<typeof BookingGuestScreen>;
export const Light: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="ada-lovelace" />),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

export const PausedPreview: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="" preview />, {
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  }),
};
export const PausedPreviewDark: Story = {
  ...PausedPreview,
  globals: { theme: "dark" },
};
export const PausedPreviewPhone: Story = {
  ...PausedPreview,
  tags: ["uat-phone"],
};
