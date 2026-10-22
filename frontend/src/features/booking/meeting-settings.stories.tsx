import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "../../testing/book.testkit";
import { jsonResponse, meRoute } from "../../testing/story-utils";
import { bookingFrame } from "./book.storykit";
import { MeetingSettings } from "./meeting-settings";

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
export const Phone: Story = {
  ...OneCalendar,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
// A seat that may not book keeps its hours, and is told why the rest is absent.
export const BookingNotGranted: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    "GET /me": meRoute({}),
  }),
};
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
// No calendar yet: the setup card leads and Accept bookings waits for one.
export const NoCalendar: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, provider: "", enabled: false }),
    "GET /connectors": () =>
      jsonResponse({
        data: [],
        providers: [{ provider: "gmail", reason: "ready" }],
      }),
  }),
};
export const NoCalendarDark: Story = {
  ...NoCalendar,
  globals: { theme: "dark" },
};

// Bookings are live and the calendar behind them has expired, so nothing
// lands in it: the links card warns and offers Reconnect beside Pause.
export const NeedsAttention: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    ...identity,
    "GET /connectors": () =>
      jsonResponse({
        data: [{ ...bookingConnection, status: "reauth_required" }],
      }),
  }),
};
export const NeedsAttentionDark: Story = {
  ...NeedsAttention,
  globals: { theme: "dark" },
};

// A member without the booking grant keeps their working hours and nothing else.
export const LowerGrant: Story = {
  render: bookingFrame(() => <MeetingSettings />, {
    "GET /me": meRoute({}),
  }),
};
export const LowerGrantDark: Story = {
  ...LowerGrant,
  globals: { theme: "dark" },
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
export const NoCalendarPhone: Story = {
  ...NoCalendar,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
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
