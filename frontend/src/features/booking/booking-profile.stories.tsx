import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { bookingProfile } from "../../testing/book.testkit";
import { jsonResponse } from "../../testing/story-utils";
import { bookingFrame } from "./book.storykit";
import { BookingProfileScreen } from "./booking-profile";

import "./book.css";

const meta: Meta<typeof BookingProfileScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Patterns/Booking/Profile page",
  component: BookingProfileScreen,
};
export default meta;
type Story = StoryObj<typeof BookingProfileScreen>;
export const Light: Story = {
  render: bookingFrame(() => <BookingProfileScreen />),
};
export const Dark: Story = { ...Light, globals: { theme: "dark" } };
export const Phone: Story = {
  ...Light,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const NotCreated: Story = {
  render: bookingFrame(() => <BookingProfileScreen />, {
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        enabled: false,
        provider: "",
        slug: undefined,
        public_url: undefined,
      }),
  }),
};
export const NotCreatedDark: Story = {
  ...NotCreated,
  globals: { theme: "dark" },
};
export const NotCreatedPhone: Story = {
  ...NotCreated,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const MissingPublicAddress: Story = {
  render: bookingFrame(() => <BookingProfileScreen />, {
    "GET /scheduling/profile": () =>
      jsonResponse({
        ...bookingProfile,
        enabled: false,
        public_url: undefined,
      }),
  }),
};
export const MissingPublicAddressDark: Story = {
  ...MissingPublicAddress,
  globals: { theme: "dark" },
};
export const MissingPublicAddressPhone: Story = {
  ...MissingPublicAddress,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// Embedded in Settings, where the page names where its identity comes from.
// The company's name starts where the host's name starts, logo or not.
async function valuesShareAnEdge({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const host = await canvas.findByText("Ada Lovelace");
  const company = await canvas.findByRole("img", { name: "Gradion" });
  const companyText = company.querySelector(".company-logo-fallback");
  if (!companyText) throw new Error("the company drew no name");
  await expect(Math.round(companyText.getBoundingClientRect().left)).toBe(
    Math.round(host.getBoundingClientRect().left),
  );
}

export const InSettings: Story = {
  render: bookingFrame(() => <BookingProfileScreen embedded />),
  play: valuesShareAnEdge,
};
export const InSettingsPhone: Story = {
  ...InSettings,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// Paused with a calendar chosen: the switch is off and free to turn on.
export const PausedInSettings: Story = {
  render: bookingFrame(() => <BookingProfileScreen embedded />, {
    "GET /scheduling/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  }),
};
export const PausedInSettingsDark: Story = {
  ...PausedInSettings,
  globals: { theme: "dark" },
};
