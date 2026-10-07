import type { Meta, StoryObj } from "@storybook/react-vite";
import { bookingFrame } from "./book.storykit";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "./book.testkit";
import { MeetingSettings } from "./meeting-settings";
import { jsonResponse, meRoute } from "./story-utils";

const meta: Meta<typeof MeetingSettings> = {
  title: "Settings/You/Meetings/Meeting settings",
  component: MeetingSettings,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MeetingSettings>;
const identity = { "GET /me": meRoute({ activity: ["create"] }) };
export const OneCalendar: Story = {
  render: bookingFrame(() => <MeetingSettings />, identity),
};
export const Dark: Story = { ...OneCalendar, globals: { theme: "dark" } };
export const Phone: Story = { ...OneCalendar, tags: ["uat-phone"] };
export const TwoProviders: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
    "GET /connectors": () =>
      jsonResponse({
        data: [
          bookingConnection,
          {
            ...bookingConnection,
            id: "outlook",
            provider: "graphcal",
            account_label: "ada@outlook.example",
            scopes: ["Calendars.ReadWrite"],
          },
        ],
      }),
  }),
};
export const ReadOnlyCalendar: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
    "GET /connectors": () =>
      jsonResponse({
        data: [
          {
            ...bookingConnection,
            scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
          },
        ],
      }),
  }),
};
export const NoCalendar: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "", enabled: false }),
    "GET /connectors": () => jsonResponse({ data: [] }),
  }),
};

// An Outlook calendar names Microsoft Teams for the default video link, and a
// host who switched it off sees the location field carry the meeting instead.
export const OutlookVideoOff: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
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
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        provider: "graphcal",
        video_call: false,
      }),
  }),
};
export const OutlookVideoOffDark: Story = {
  ...OutlookVideoOff,
  globals: { theme: "dark" },
};
export const SetupNotFinished: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /me/working-hours": () =>
      jsonResponse({
        chosen: false,
        working_hours: bookingHours.working_hours,
      }),
  }),
};
