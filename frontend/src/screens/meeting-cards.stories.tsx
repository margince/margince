import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import { bookingConnection, bookingProfile } from "./book.testkit";
import { AvailabilityCard } from "./meeting-availability";
import { MeetingCalendarCard } from "./meeting-calendar";
import { GuestViewCard } from "./meeting-guest-view";
import { BookingLinksCard } from "./meeting-links";
import { MeetingSetupCard } from "./meeting-setup";
import { jsonResponse } from "./story-utils";

// Each Meetings card alone, in the states the page composes them in. The
// whole page is in meeting-settings.stories.tsx.
const meta: Meta = {
  title: "Settings/You/Meetings/Cards",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const expired = {
  "GET /connectors": () =>
    jsonResponse({
      data: [{ ...bookingConnection, status: "reauth_required" }],
    }),
};

export const Setup: Story = {
  render: bookingFrame(() => <MeetingSetupCard hoursChosen={false} hasLink />, {
    "GET /connectors": () =>
      jsonResponse({
        data: [],
        providers: [{ provider: "gmail", reason: "ready" }],
      }),
  }),
};
export const SetupDark: Story = { ...Setup, globals: { theme: "dark" } };

export const BookingLinks: Story = {
  render: bookingFrame(() => (
    <BookingLinksCard profile={bookingProfile} lead />
  )),
};
export const BookingLinksDark: Story = {
  ...BookingLinks,
  globals: { theme: "dark" },
};
export const BookingLinksNeedsAttention: Story = {
  render: bookingFrame(
    () => <BookingLinksCard profile={bookingProfile} lead />,
    expired,
  ),
};
export const BookingLinksNeedsAttentionDark: Story = {
  ...BookingLinksNeedsAttention,
  globals: { theme: "dark" },
};
export const NoLinkYet: Story = {
  render: bookingFrame(() => (
    <BookingLinksCard
      profile={{ ...bookingProfile, slug: undefined, public_url: undefined }}
      lead={false}
    />
  )),
};

export const Calendar: Story = {
  render: bookingFrame(() => <MeetingCalendarCard profile={bookingProfile} />),
};
export const CalendarDark: Story = { ...Calendar, globals: { theme: "dark" } };
export const CalendarExpired: Story = {
  render: bookingFrame(
    () => <MeetingCalendarCard profile={bookingProfile} />,
    expired,
  ),
};
export const CalendarExpiredDark: Story = {
  ...CalendarExpired,
  globals: { theme: "dark" },
};

export const Availability: Story = {
  render: bookingFrame(() => <AvailabilityCard profile={bookingProfile} />),
};
export const AvailabilityDark: Story = {
  ...Availability,
  globals: { theme: "dark" },
};
export const HoursOnly: Story = {
  render: bookingFrame(() => <AvailabilityCard hoursOnly />),
};

export const GuestView: Story = {
  render: bookingFrame(() => <GuestViewCard profile={bookingProfile} />),
};
export const GuestViewDark: Story = {
  ...GuestView,
  globals: { theme: "dark" },
};
