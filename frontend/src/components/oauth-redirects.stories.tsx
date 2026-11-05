// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import {
  expectFoldedRowsUnderSummary,
  StoryProviders,
} from "../testing/story-utils";
import { RedirectUriGroup, RedirectUris } from "./oauth-redirects";

// The callback addresses on their own, two ways. In a vendor's panel they fold
// behind a summary with their count, open until the vendor has an app. Bare,
// they render as the first-run step draws them.

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
        <PanelBody>
          <RedirectUriGroup uris={URIS} sub={SUB} open={false} />
        </PanelBody>
      </Panel>
    </StoryProviders>
  ),
};

export const InAPanelNoApp: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="Google app">
        <PanelBody>
          <RedirectUriGroup uris={URIS} sub={SUB} open />
        </PanelBody>
      </Panel>
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table");
    await expect(
      canvas.getByText("Authorized redirect URIs").closest("summary"),
    ).toHaveAttribute("aria-expanded", "true");
  },
};

export const InAPanelNoAppPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: InAPanelNoApp.render,
  play: InAPanelNoApp.play,
};

const openFold = async ({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) => {
  await userEvent.click(
    await within(canvasElement).findByText("Authorized redirect URIs"),
  );
  await within(canvasElement).findByRole("table");
};

export const InAPanelOpen: Story = {
  render: InAPanel.render,
  play: openFold,
};

// Folded, each address row starts where the summary above it starts.
export const InAPanelPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: InAPanel.render,
  play: async (context) => {
    await openFold(context);
    await expectFoldedRowsUnderSummary(context.canvasElement);
  },
};

export const Bare: Story = {
  render: () => (
    <StoryProviders>
      <RedirectUris uris={URIS} sub={SUB} />
    </StoryProviders>
  ),
};
