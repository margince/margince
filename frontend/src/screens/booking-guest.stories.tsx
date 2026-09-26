import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { BookingGuestScreen } from "./booking-guest";

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
