import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { userEvent, within } from "storybook/test";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { bookingConnection, bookingProfile } from "../../testing/book.testkit";
import { jsonResponse } from "../../testing/story-utils";
import { bookingFrame } from "./book.storykit";
import { AvailabilityCard } from "./meeting-availability";
import { MeetingCalendarCard } from "./meeting-calendar";
import { GuestViewCard } from "./meeting-guest-view";
import { BookingLinksCard } from "./meeting-links";
import { MeetingSetupCard } from "./meeting-setup";

// Each Meetings card alone, in the states the page composes them in. The
// whole page is in meeting-settings.stories.tsx.
// A card's draft saves through the page's bar, as it does in settings.
function onPage(card: ReactNode) {
  return (
    <SavePageProvider>
      {card}
      <PageSaveBar />
    </SavePageProvider>
  );
}

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
export const SetupPhone: Story = {
  ...Setup,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const refused = () =>
  jsonResponse({ title: "Service unavailable", status: 503 }, 503);

/** A connect press the server refused: the reason sits under the steps. */
export const ConnectRefusal: Story = {
  render: bookingFrame(() => <MeetingSetupCard hoursChosen={false} hasLink />, {
    "GET /connectors": () =>
      jsonResponse({
        data: [],
        providers: [{ provider: "gmail", reason: "ready" }],
      }),
    "POST /connectors/gcal/connect": refused,
    "POST /connectors/graphcal/connect": refused,
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /Connect/ }),
    );
  },
};

export const BookingLinks: Story = {
  render: bookingFrame(() => <BookingLinksCard profile={bookingProfile} />),
};
export const BookingLinksDark: Story = {
  ...BookingLinks,
  globals: { theme: "dark" },
};
export const BookingLinksNeedsAttention: Story = {
  render: bookingFrame(
    () => <BookingLinksCard profile={bookingProfile} />,
    expired,
  ),
};
export const BookingLinksPhone: Story = {
  ...BookingLinks,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const BookingLinksNeedsAttentionDark: Story = {
  ...BookingLinksNeedsAttention,
  globals: { theme: "dark" },
};
export const NoLinkYet: Story = {
  render: bookingFrame(() => (
    <BookingLinksCard
      profile={{ ...bookingProfile, slug: undefined, public_url: undefined }}
    />
  )),
};

export const Calendar: Story = {
  render: bookingFrame(() =>
    onPage(<MeetingCalendarCard profile={bookingProfile} />),
  ),
};
export const CalendarDark: Story = { ...Calendar, globals: { theme: "dark" } };
export const CalendarExpired: Story = {
  render: bookingFrame(
    () => onPage(<MeetingCalendarCard profile={bookingProfile} />),
    expired,
  ),
};
export const CalendarExpiredDark: Story = {
  ...CalendarExpired,
  globals: { theme: "dark" },
};

export const Availability: Story = {
  render: bookingFrame(() =>
    onPage(<AvailabilityCard profile={bookingProfile} />),
  ),
};
export const AvailabilityDark: Story = {
  ...Availability,
  globals: { theme: "dark" },
};
/** A ticked day makes the card dirty: Discard and Save appear at its foot. */
export const AvailabilityDirty: Story = {
  render: bookingFrame(() =>
    onPage(<AvailabilityCard profile={bookingProfile} />),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("checkbox", { name: "Saturday" }),
    );
  },
};
/** The hours write refused: the save bar says so once. */
export const AvailabilityRefused: Story = {
  render: bookingFrame(
    () => onPage(<AvailabilityCard profile={bookingProfile} />),
    {
      "PUT /me/working-hours": refused,
    },
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("checkbox", { name: "Saturday" }),
    );
    await userEvent.click(await canvas.findByRole("button", { name: "Save" }));
  },
};
export const HoursOnly: Story = {
  render: bookingFrame(() => onPage(<AvailabilityCard hoursOnly />)),
};

export const GuestView: Story = {
  render: bookingFrame(() => <GuestViewCard profile={bookingProfile} />),
};
export const GuestViewDark: Story = {
  ...GuestView,
  globals: { theme: "dark" },
};
