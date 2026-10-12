import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { viewerZone } from "../format/timezone";
import { bookingFrame } from "./book.storykit";
import { bookingSlots } from "./book.testkit";
import { BookingBlocked, BookingPicker } from "./booking-picker";
import { useSchedulingProfile } from "./scheduling-profile-query";
import { useWorkingHours } from "./working-hours";

import "./book.css";

// The week a host picks from renders inside the invite page's stories; these
// are the week a personal link shows instead, and the state that stands in for
// it before a calendar is set up.
const meta: Meta<typeof BookingPicker> = {
  title: "Patterns/Booking/Time picker",
  component: BookingPicker,
};
export default meta;
type Story = StoryObj<typeof BookingPicker>;

// The open times the guest will choose from, with the length the link books.
function OpenTimes() {
  const hours = useWorkingHours(true);
  const profile = useSchedulingProfile(true);
  const [duration, setDuration] = useState(30);
  // The fixture's own week, so the story draws the same days whatever the date.
  const [from, setFrom] = useState(bookingSlots[0].start);
  const [searchAhead, setSearchAhead] = useState(false);
  return (
    <BookingPicker
      mode="link"
      configured={profile.isSuccess}
      profile={profile.data}
      hours={hours}
      zone={viewerZone()}
      onZone={() => undefined}
      duration={duration}
      onDuration={setDuration}
      from={from}
      onFrom={setFrom}
      searchAhead={searchAhead}
      onSearchAhead={setSearchAhead}
      picks={[]}
      onPick={() => undefined}
    />
  );
}

export const PersonalLinkOpenTimes: Story = {
  render: bookingFrame(() => <OpenTimes />),
};
export const PersonalLinkOpenTimesDark: Story = {
  ...PersonalLinkOpenTimes,
  globals: { theme: "dark" },
};
export const CalendarBlocked: Story = {
  render: bookingFrame(() => <BookingBlocked />),
};
