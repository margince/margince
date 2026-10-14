// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { MailSharingPostureRow } from "./mail-sharing";

// The sharing rule as stated on a member's own Connections page. The answer is
// a sentence under the row's name. A reader who may change it gets the link.
function story(mail_sharing: boolean, allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/settings": () =>
        jsonResponse({ auto_enrich: true, mail_sharing }),
    });
    return (
      <StoryProviders>
        <MailSharingPostureRow />
      </StoryProviders>
    );
  };
}

const MANAGER = { capture_settings: ["read", "update"] } as const;
const READER = { capture_settings: ["read"] } as const;

const meta: Meta<typeof MailSharingPostureRow> = {
  title: "Settings/You/Connections/Email sharing",
  component: MailSharingPostureRow,
};
export default meta;
type Story = StoryObj<typeof MailSharingPostureRow>;

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
