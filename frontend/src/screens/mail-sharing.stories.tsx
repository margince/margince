// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MailSharingCard } from "./mail-sharing";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The workspace mail-sharing posture and the sync interval beside it. On is the
// quiet default. Off draws the danger callout, the one place that states what
// turning sharing off costs.
function story(mail_sharing: boolean, allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/settings": () =>
        jsonResponse({
          auto_enrich: true,
          mail_sharing,
          shared_posture_allowed: false,
          signature_enrich: true,
          mail_sync_interval_seconds: 120,
        }),
      "PATCH /capture/settings": (body) =>
        jsonResponse({ mail_sharing, ...(body as object) }),
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
