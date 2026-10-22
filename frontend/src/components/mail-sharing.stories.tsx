// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  permissionDenied,
  StoryProviders,
} from "../testing/story-utils";
import { MailSharingCard } from "./mail-sharing";

// On is the quiet default. A posture with a cost (sharing off, or seats allowed
// to share) states it under the switches. Each switch writes on flip, so the
// other state is a refused write.
type CaptureSettings = components["schemas"]["CaptureSettings"];

function story(
  mail_sharing: boolean,
  allow: Parameters<typeof meRoute>[0],
  refuseWrites = false,
  sharedPostureAllowed = false,
) {
  return () => {
    let settings: CaptureSettings = {
      auto_enrich: true,
      auto_enrich_daily_cap: 500,
      mail_sharing,
      shared_posture_allowed: sharedPostureAllowed,
      signature_enrich: true,
      site_read: { max_pages: 60, max_mib: 32, wall_seconds: 240 },
      mail_sync_interval_seconds: 120,
      confidentiality_pass_cap: 40,
    };
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/settings": () => jsonResponse(settings),
      "PATCH /capture/settings": (body) => {
        if (refuseWrites) return permissionDenied();
        settings = Object.assign({}, settings, body);
        return jsonResponse(settings);
      },
    });
    return (
      <StoryProviders>
        <MailSharingCard />
      </StoryProviders>
    );
  };
}

const MANAGER = { capture_settings: ["read", "update"] } as const;
const READER = { capture_settings: ["read"] } as const;

const meta = {
  title: "Settings/Data/Capture rules/Mail sharing and sync",
  component: MailSharingCard,
} satisfies Meta<typeof MailSharingCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SharingOn: Story = { render: story(true, MANAGER) };

// The stored posture IS off: the danger callout stands without any
// interaction, so an admin landing here later still sees the cost stated.
export const SharingOff: Story = { render: story(false, MANAGER) };

export const CannotChange: Story = { render: story(true, READER) };

export const CannotChangeDark: Story = {
  globals: { theme: "dark" },
  render: story(true, READER),
};

// Three rows at 390px: each control drops under its words, none clipped.
export const SharingOnPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(true, MANAGER),
};

export const SyncRefused: Story = {
  render: story(true, MANAGER, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const box = await canvas.findByTestId("capture-mail-sync");
    await userEvent.clear(box);
    await userEvent.type(box, "60{Enter}");
    await canvas.findByText("Setting not changed");
    await expect(box).toHaveAccessibleDescription(
      expect.stringContaining("Setting not changed"),
    );
  },
};

// Seats may put their mailbox in the shared posture: the works-council warning
// stands under the switches for as long as that is allowed.
export const SharedPostureAllowed: Story = {
  render: story(true, MANAGER, false, true),
};

// A flip the server refused: one notice for the card, and the switch shows the
// stored posture again.
export const SaveRefused: Story = {
  render: story(true, MANAGER, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("mail-sharing-toggle"));
    await canvas.findByText("Setting not saved");
  },
};
