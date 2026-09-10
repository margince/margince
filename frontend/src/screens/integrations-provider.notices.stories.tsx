// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { ProblemError } from "./common";
import {
  BuyableRefused,
  PostureRefused,
} from "./integrations-provider.notices";
import { StoryProviders } from "./story-utils";

// The two refused-switch bands, in the list they belong to: a non-row child of
// `SettingList`, so the plate pays for its own interval and draws no hairline.
// The read-failure frame is the one a real card can only reach by breaking the
// provider's own endpoint.

const meta: Meta = {
  title: "Settings/Data/Integrations/Provider notices",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function refused(detail: string) {
  return new ProblemError({
    type: "about:blank",
    title: "Bad Gateway",
    status: 502,
    detail,
  });
}

const WRITE = refused("The provider refused the change. Try again shortly.");
const READ = refused("The current setting could not be read.");

function Frame({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <StoryProviders>
      <div className="settinglist">{children}</div>
    </StoryProviders>
  );
}

/** The flip did not take: the heading names the setting, the server says why. */
export const LookupWriteRefused: Story = {
  render: () => (
    <Frame>
      <PostureRefused writeError={WRITE} readError={null} />
    </Frame>
  ),
};

/**
 * The READ failed, so "off" is a claim the card cannot make. Its own heading,
 * because a reader must be able to tell this from a press that was refused.
 */
export const LookupReadRefused: Story = {
  render: () => (
    <Frame>
      <PostureRefused writeError={null} readError={READ} />
    </Frame>
  ),
};

/** One priced category's buy switch, refused. */
export const BuySwitchRefused: Story = {
  render: () => (
    <Frame>
      <BuyableRefused error={WRITE} />
    </Frame>
  ),
};

/** The same band in dark: tone reaches the heading's ink and nothing else. */
export const BuySwitchRefusedDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <Frame>
      <BuyableRefused error={WRITE} />
    </Frame>
  ),
};

/** Nothing refused, nothing drawn — the caller states no condition of its own. */
export const Quiet: Story = {
  render: () => (
    <Frame>
      <PostureRefused writeError={null} readError={null} />
      <BuyableRefused error={null} />
    </Frame>
  ),
};
