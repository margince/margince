// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MailSharingPlate } from "../../components/mail-sharing";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";

// The sharing rule as stated on a member's own Connections page, as the plate
// under their accounts. A reader who may change it gets the link.
function story(mail_sharing: boolean, allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/settings": () =>
        jsonResponse({ auto_enrich: true, mail_sharing }),
    });
    return (
      <StoryProviders>
        <MailSharingPlate />
      </StoryProviders>
    );
  };
}

const MANAGER = { capture_settings: ["read", "update"] } as const;
const READER = { capture_settings: ["read"] } as const;

const meta: Meta<typeof MailSharingPlate> = {
  title: "Settings/You/Connections/Email sharing",
  component: MailSharingPlate,
};
export default meta;
type Story = StoryObj<typeof MailSharingPlate>;

export const SharedForAManager: Story = { render: story(true, MANAGER) };

export const PrivateForAReader: Story = { render: story(false, READER) };

export const SharedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(true, MANAGER),
};

export const SharedDark: Story = {
  globals: { theme: "dark" },
  render: story(true, MANAGER),
};
