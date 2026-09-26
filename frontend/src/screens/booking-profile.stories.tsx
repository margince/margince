import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { BookingProfileScreen } from "./booking-profile";

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
