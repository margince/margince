// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { PasswordLinkModal } from "./users-password-link";

// The one-time link an admin hands a member who cannot sign in. Prop-driven, so
// the three states it can be in are three sets of props rather than three
// server fixtures — and all three matter: the link is shown once, so the
// pending and failed states are what a reader sees when it is not.

// A full-length token, because a short one fits any box.
const LINK = {
  url: "https://margince.example/#/reset-password?token=Qm9vdHN0cmFwLXRva2VuLWZvci1kYW5hLWtlc3NsZXItMjAyNg",
  expiresAt: "2026-08-15T09:00:00Z",
};

// The whole link is on screen: nothing waits past the block's right edge.
async function linkFits({ canvasElement }: { canvasElement: HTMLElement }) {
  const body = within(canvasElement.ownerDocument.body);
  const link = await body.findByTestId("password-link-url");
  await expect(link.scrollWidth).toBeLessThanOrEqual(link.clientWidth);
}

const meta: Meta<typeof PasswordLinkModal> = {
  title: "Settings/People/Members/Password link",
  component: PasswordLinkModal,
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof PasswordLinkModal>;

export const Minted: Story = {
  play: linkFits,
  render: () => (
    <PasswordLinkModal
      onClose={() => undefined}
      memberName="Dana Kessler"
      link={LINK}
      pending={false}
      error={null}
      onRetry={() => undefined}
    />
  ),
};

// The narrow case: a live account-takeover credential the admin may have to
// read off the screen to dictate, so a clipped URL is the failure that matters.
// On a phone the confirm stays a card over the scrim, narrower still.
export const MintedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: linkFits,
  render: () => (
    <PasswordLinkModal
      onClose={() => undefined}
      memberName="Dana Kessler"
      link={LINK}
      pending={false}
      error={null}
      onRetry={() => undefined}
    />
  ),
};

export const Minting: Story = {
  render: () => (
    <PasswordLinkModal
      onClose={() => undefined}
      memberName="Dana Kessler"
      link={null}
      pending
      error={null}
      onRetry={() => undefined}
    />
  ),
};

export const Failed: Story = {
  render: () => (
    <PasswordLinkModal
      onClose={() => undefined}
      memberName="Dana Kessler"
      link={null}
      pending={false}
      error="The link could not be minted. Try again."
      onRetry={() => undefined}
    />
  ),
};
