// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../design-system/panel";
import { RedirectUriGroup, RedirectUris } from "./oauth-redirects";
import { StoryProviders } from "./story-utils";

// The callback addresses on their own: as a section of a vendor's panel, where
// the table runs edge to edge, and bare, as the first-run step draws them.

const SUB =
  "Register every URI below on the OAuth client in the Google console.";

const URIS = [
  {
    purpose: "sign_in",
    url: "https://api.brandt.example/v1/auth/oidc/google/callback",
  },
  {
    purpose: "mailbox_connect",
    url: "https://crm.brandt-automotive-holding-international.example:8443/eu-central-1/v1/connectors/gmail/callback",
  },
  {
    purpose: "calendar_connect",
    url: "https://api.brandt.example/v1/connectors/gcal/callback",
  },
];

const meta: Meta<typeof RedirectUriGroup> = {
  title: "Settings/Company/Sign-in and apps/Redirect addresses",
  component: RedirectUriGroup,
};
export default meta;
type Story = StoryObj<typeof RedirectUriGroup>;

export const InAPanel: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="Google app">
        <RedirectUriGroup uris={URIS} sub={SUB} />
      </Panel>
    </StoryProviders>
  ),
};

export const InAPanelPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: InAPanel.render,
};

export const Bare: Story = {
  render: () => (
    <StoryProviders>
      <RedirectUris uris={URIS} sub={SUB} />
    </StoryProviders>
  ),
};
