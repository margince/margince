import type { Meta, StoryObj } from "@storybook/react-vite";
import { viewerZone } from "../format/timezone";
import { bookingContact, bookingSlots } from "./book.testkit";
import { type BookingDraft, BookingReview } from "./booking-review";
import { StoryProviders } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingReview> = {
  title: "Patterns/Booking/Review panel",
  component: BookingReview,
};
export default meta;
type Story = StoryObj<typeof BookingReview>;

const draft: BookingDraft = {
  contactId: bookingContact.id,
  contactName: bookingContact.full_name,
  attendee: bookingContact.primary_email,
  subject: "Intro call",
  location: "",
  description: "",
  duration: 30,
  video: true,
  provider: "gcal",
};
const panel = (props: Partial<Parameters<typeof BookingReview>[0]>) => () => (
  <StoryProviders>
    <div style={{ maxWidth: 380 }}>
      <BookingReview
        mode="propose"
        draft={draft}
        onEdit={() => undefined}
        picks={bookingSlots}
        onRemovePick={() => undefined}
        configured
        zone={viewerZone()}
        searchContacts={async () => []}
        {...props}
      />
    </div>
  </StoryProviders>
);

export const TwoTimesOffered: Story = { render: panel({}) };
export const TwoTimesOfferedDark: Story = {
  ...TwoTimesOffered,
  globals: { theme: "dark" },
};
export const AgreedTimeWithoutVideo: Story = {
  render: panel({
    mode: "invite",
    picks: bookingSlots.slice(0, 1),
    draft: { ...draft, video: false, location: "Office, Room 2" },
  }),
};
export const PersonalLinkOnOutlook: Story = {
  render: panel({
    mode: "link",
    picks: [],
    draft: { ...draft, provider: "graphcal" },
  }),
};
export const CalendarNotReady: Story = {
  render: panel({ configured: false, picks: [] }),
};
