import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { bookingConnection, bookingProfile } from "../../testing/book.testkit";
import { jsonResponse } from "../../testing/story-utils";
import { bookingFrame } from "./book.storykit";
import {
  MeetingLinkDrawer,
  ReplaceAddressConfirm,
} from "./meeting-link-drawer";

const meta: Meta<typeof MeetingLinkDrawer> = {
  title: "Settings/You/Meetings/Booking link drawer",
  component: MeetingLinkDrawer,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof MeetingLinkDrawer>;

const drawer =
  (profile = bookingProfile) =>
  () => (
    <MeetingLinkDrawer
      open
      onClose={fn()}
      onChangeHours={fn()}
      profile={profile}
    />
  );

// A Google calendar: the Meet tile is on and Teams asks for a Microsoft one.
export const GoogleCalendar: Story = {
  render: bookingFrame(drawer({ ...bookingProfile, video_call: true })),
};
export const GoogleCalendarDark: Story = {
  ...GoogleCalendar,
  globals: { theme: "dark" },
};
export const Phone: Story = { ...GoogleCalendar, tags: ["uat-phone"] };

// The host's own room, with a length off the presets that opens on Other.
export const OwnAddressOtherLength: Story = {
  render: bookingFrame(
    drawer({
      ...bookingProfile,
      provider: "graphcal",
      video_call: false,
      location: "https://zoom.example/j/123",
      duration_minutes: 75,
    }),
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              ...bookingConnection,
              provider: "graphcal",
              scopes: ["Calendars.ReadWrite"],
            },
          ],
        }),
    },
  ),
};
export const OwnAddressOtherLengthDark: Story = {
  ...OwnAddressOtherLength,
  globals: { theme: "dark" },
};

export const ReplaceAddress: Story = {
  render: bookingFrame(() => <ReplaceAddressConfirm open onClose={fn()} />),
};
export const ReplaceAddressDark: Story = {
  ...ReplaceAddress,
  globals: { theme: "dark" },
};
