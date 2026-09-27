import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { BookingSetup } from "./booking-setup";
import "./book.css";

const meta: Meta<typeof BookingSetup> = {
  title: "Patterns/Booking/Calendar setup",
  component: BookingSetup,
};
export default meta;
type Story = StoryObj<typeof BookingSetup>;
export const SetupRequired: Story = {
  render: bookingFrame(() => <BookingSetup />),
};
export const Dark: Story = { ...SetupRequired, globals: { theme: "dark" } };
