import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { viewerZone } from "../format/timezone";
import { bookingFrame } from "./book.storykit";
import { bookingProfile } from "./book.testkit";
import { BookingGuestScreen } from "./booking-guest";
import { GuestDetailsForm } from "./booking-guest-parts";
import { jsonResponse, type RouteMap } from "./story-utils";

import "./book.css";

const meta: Meta<typeof BookingGuestScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/Guest page",
  component: BookingGuestScreen,
};
export default meta;
type Story = StoryObj<typeof BookingGuestScreen>;
// The catalogue's free times fall a few days on rather than on a fixed date
// that drifts into the past. Late in a month that is next month, which the
// page, opening on the month it is viewed in, reaches by one press.
function openTimes() {
  const now = new Date();
  return [2, 3, 5].flatMap((ahead) => {
    const day = new Date(now);
    day.setDate(now.getDate() + ahead);
    return [9, 11, 14].map((hour) => {
      const start = new Date(day);
      start.setHours(hour, 0, 0, 0);
      return {
        start: start.toISOString(),
        end: new Date(start.getTime() + 30 * 60000).toISOString(),
      };
    });
  });
}
const timesThisMonth = () =>
  openTimes().some(
    (slot) => new Date(slot.start).getMonth() === new Date().getMonth(),
  );
const openMonth: RouteMap = {
  "GET /public/booking/ada-lovelace/availability": () =>
    jsonResponse({ slots: openTimes(), truncated: false }),
  "GET /availability": () =>
    jsonResponse({ slots: openTimes(), truncated: false }),
};

export const Light: Story = {
  render: bookingFrame(
    () => <BookingGuestScreen hostSlug="ada-lovelace" />,
    openMonth,
  ),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

// A time picked: it moves to the summary on the left, and the details form
// takes the times' place with "Change time" beside the confirmation.
export const PickedTime: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="ada-lovelace" />, {
    ...openMonth,
    "GET /public/booking/ada-lovelace/profile": () =>
      jsonResponse({ ...bookingProfile, video_app: "google_meet" }),
  }),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    if (!timesThisMonth())
      await userEvent.click(
        await page.findByRole("button", { name: "Next month" }),
      );
    const [first] = await page.findAllByRole("button", {
      name: /^\d{1,2}:\d{2}/,
    });
    await userEvent.click(first);
  },
};
export const PickedTimeDark: Story = {
  ...PickedTime,
  globals: { theme: "dark" },
};
export const PickedTimePhone: Story = {
  ...PickedTime,
  tags: ["uat-phone"],
};

export const PausedPreview: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="" preview />, {
    ...openMonth,
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  }),
};
export const PausedPreviewDark: Story = {
  ...PausedPreview,
  globals: { theme: "dark" },
};
export const PausedPreviewPhone: Story = {
  ...PausedPreview,
  tags: ["uat-phone"],
};

export const PreviewEmpty: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="" preview />, {
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () => jsonResponse({ slots: [], truncated: false }),
  }),
};
export const PreviewCalendarError: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="" preview />, {
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
    "GET /availability": () =>
      jsonResponse(
        {
          title: "Calendar unavailable",
          status: 503,
          detail: "Reconnect your calendar in Meetings settings and try again.",
        },
        503,
      ),
  }),
};

export const PreviewEmptyDark: Story = {
  ...PreviewEmpty,
  globals: { theme: "dark" },
};
export const PreviewEmptyPhone: Story = {
  ...PreviewEmpty,
  tags: ["uat-phone"],
};
export const PreviewCalendarErrorDark: Story = {
  ...PreviewCalendarError,
  globals: { theme: "dark" },
};
export const PreviewCalendarErrorPhone: Story = {
  ...PreviewCalendarError,
  tags: ["uat-phone"],
};
export const PreviewSetup: Story = {
  render: bookingFrame(() => <BookingGuestScreen hostSlug="" preview />, {
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false, provider: "" }),
  }),
};
export const PreviewSetupDark: Story = {
  ...PreviewSetup,
  globals: { theme: "dark" },
};
export const PreviewSetupPhone: Story = {
  ...PreviewSetup,
  tags: ["uat-phone"],
};

// A personal proposal's details: the link already names its recipient, so the
// form asks only for consent before the confirmation it enables.
export const PersonalDetails: Story = {
  render: bookingFrame(() => (
    <GuestDetailsForm
      selected={{
        start: "2026-10-05T09:00:00Z",
        end: "2026-10-05T09:30:00Z",
      }}
      zone={viewerZone()}
      personal
      details={{ name: "", email: "", topic: "", consent: true }}
      onDetails={() => {}}
      onChangeTime={() => {}}
      onSubmit={() => {}}
      refused={false}
      pending={false}
      error={null}
    />
  )),
};
