import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { bookingFrame } from "./book.storykit";
import { bookingProfile } from "./book.testkit";
import { BookingProfileScreen } from "./booking-profile";
import { jsonResponse } from "./story-utils";

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
export const Phone: Story = { ...Light, tags: ["uat-phone"] };

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
export const NotCreatedPhone: Story = { ...NotCreated, tags: ["uat-phone"] };
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
  tags: ["uat-phone"],
};
