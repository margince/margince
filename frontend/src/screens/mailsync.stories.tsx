// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MailSyncCard } from "./capture-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// How often a mailbox syncs, editable and refused.
function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    let settings = { mail_sync_interval_seconds: 120 };
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/settings": () => jsonResponse(settings),
      "PATCH /capture/settings": (body) => {
        settings = { ...settings, ...(body as object) };
        return jsonResponse(settings);
      },
    });
    return (
      <StoryProviders>
        <MailSyncCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof MailSyncCard> = {
  title: "Settings/Data/Capture rules/Mail sync",
  component: MailSyncCard,
};
export default meta;
type Story = StoryObj<typeof MailSyncCard>;

export const Editable: Story = {
  render: story({ capture_settings: ["read", "update"] }),
};

export const CannotChange: Story = {
  render: story({ capture_settings: ["read"] }),
};

export const CannotChangeDark: Story = {
  globals: { theme: "dark" },
  render: story({ capture_settings: ["read"] }),
};
