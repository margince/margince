import type { Meta, StoryObj } from "@storybook/react-vite";
import { UTC_ZONE } from "../../format/timezone";
import { StoryProviders } from "../../testing/story-utils";
import { BookingZone } from "./booking-common";
import "./book.css";

const meta: Meta = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/Shared booking controls",
};
export default meta;
export const Light: StoryObj = {
  render: () => (
    <StoryProviders>
      <div className="book-guest-page">
        <BookingZone value={UTC_ZONE} onChange={() => {}} />
      </div>
    </StoryProviders>
  ),
};
export const Dark: StoryObj = { ...Light, globals: { theme: "dark" } };
