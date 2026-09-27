import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingProfile } from "./book.testkit";
import { BookingProfileScreen } from "./booking-profile";
import { jsonResponse } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingProfileScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/BookingProfileScreen",
  component: BookingProfileScreen,
};
export default meta;
type Story = StoryObj<typeof BookingProfileScreen>;
export const Light: Story = {
  render: bookingFrame(() => <BookingProfileScreen />),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

export const NotCreated: Story = {
  render: bookingFrame(() => <BookingProfileScreen />, {
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        enabled: false,
        provider: "",
        slug: undefined,
        public_url: undefined,
      }),
  }),
};
export const NotCreatedDark: Story = {
  ...NotCreated,
  globals: { theme: "dark" },
};
export const NotCreatedPhone: Story = { ...NotCreated, tags: ["uat-phone"] };
export const MissingPublicAddress: Story = {
  render: bookingFrame(() => <BookingProfileScreen />, {
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        enabled: false,
        public_url: undefined,
      }),
  }),
};
export const MissingPublicAddressDark: Story = {
  ...MissingPublicAddress,
  globals: { theme: "dark" },
};
export const MissingPublicAddressPhone: Story = {
  ...MissingPublicAddress,
  tags: ["uat-phone"],
};
