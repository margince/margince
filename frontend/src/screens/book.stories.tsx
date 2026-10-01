import type { Meta, StoryObj } from "@storybook/react-vite";
import { BookingScreen } from "./book";
import { bookingStory } from "./book.storykit";
import {
  bookingContact,
  bookingInvitation,
  bookingProfile,
} from "./book.testkit";
import { jsonResponse } from "./story-utils";

const meta: Meta<typeof BookingScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Signed out/Booking page",
  component: BookingScreen,
};
export default meta;
type Story = StoryObj<typeof BookingScreen>;

export const PublicSlotsOffered: Story = {
  render: bookingStory("ada-lovelace"),
};
export const PublicSlotsOfferedDark: Story = {
  ...PublicSlotsOffered,
  globals: { theme: "dark" },
};
export const PublicNoAvailability: Story = {
  render: bookingStory("ada-lovelace", {
    "GET /public/booking/ada-lovelace/availability": () =>
      jsonResponse({ slots: [], truncated: false }),
  }),
};
export const PublicNoAvailabilityDark: Story = {
  ...PublicNoAvailability,
  globals: { theme: "dark" },
};
export const Paused: Story = {
  render: bookingStory("ada-lovelace", {
    "GET /public/booking/ada-lovelace/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  }),
};
export const PausedDark: Story = { ...Paused, globals: { theme: "dark" } };
export const Pending: Story = { render: bookingStory("manage-private-link") };
export const PendingDark: Story = { ...Pending, globals: { theme: "dark" } };
export const Confirmed: Story = {
  render: bookingStory("manage-private-link", {
    "GET /public/meeting/private-link": () =>
      jsonResponse({ ...bookingInvitation, status: "confirmed" }),
  }),
};
export const ConfirmedDark: Story = {
  ...Confirmed,
  globals: { theme: "dark" },
};
export const NeedsAttention: Story = {
  render: bookingStory("manage-private-link", {
    "GET /public/meeting/private-link": () =>
      jsonResponse({ ...bookingInvitation, status: "needs_attention" }),
  }),
};
export const NeedsAttentionDark: Story = {
  ...NeedsAttention,
  globals: { theme: "dark" },
};
export const Canceled: Story = {
  render: bookingStory("manage-private-link", {
    "GET /public/meeting/private-link": () =>
      jsonResponse({ ...bookingInvitation, status: "canceled" }),
  }),
};
export const CanceledDark: Story = { ...Canceled, globals: { theme: "dark" } };
export const PersonalProposal: Story = {
  render: bookingStory("proposal-personal-link"),
};
export const PersonalProposalDark: Story = {
  ...PersonalProposal,
  globals: { theme: "dark" },
};
export const MyBookingLink: Story = { render: bookingStory() };
export const MyBookingLinkDark: Story = {
  ...MyBookingLink,
  globals: { theme: "dark" },
};
export const ArrangeMeeting: Story = {
  render: bookingStory(`contact-${bookingContact.id}`),
};
export const ArrangeMeetingDark: Story = {
  ...ArrangeMeeting,
  globals: { theme: "dark" },
};

export const RecoveredProposal: Story = {
  render: bookingStory("proposal-personal-link", {
    "GET /public/proposal/personal-link": () =>
      jsonResponse({
        profile: { ...bookingProfile, enabled: false },
        used: true,
        options: [],
        description: "Project discovery",
        expires_at: "2026-10-06T09:00:00Z",
        meeting: { ...bookingInvitation, management_token: "private-link" },
      }),
  }),
};
export const RecoveredProposalDark: Story = {
  ...RecoveredProposal,
  globals: { theme: "dark" },
};
