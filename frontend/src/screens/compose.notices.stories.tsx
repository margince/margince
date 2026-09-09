// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StaleThreadNotice, VoiceDegradedNotice } from "./compose.notices";
import { StoryProviders } from "./story-utils";

// The two notices the composer says about the draft rather than about the
// message. Their own frames because reaching either in the composer itself
// takes a degraded voice profile or a deleted thread — neither of which a
// fixture can pose, and both of which a reader has to be able to recognise.

const meta: Meta = {
  title: "Patterns/Compose mail/Draft notices",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** The draft came back in nobody's voice — the one loss the text will not show. */
export const VoiceDegraded: Story = {
  render: () => (
    <StoryProviders>
      <VoiceDegradedNotice degraded />
    </StoryProviders>
  ),
};

/** The thread the reader asked to answer is gone, and the composer says so. */
export const ThreadGone: Story = {
  render: () => (
    <StoryProviders>
      <StaleThreadNotice stale />
    </StoryProviders>
  ),
};

/** Both in dark: same claim, and tone reaches the heading's ink only. */
export const BothDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      <div className="form-stack">
        <VoiceDegradedNotice degraded />
        <StaleThreadNotice stale />
      </div>
    </StoryProviders>
  ),
};

/** The ordinary draft: neither fact is true, so neither band is drawn. */
export const Quiet: Story = {
  render: () => (
    <StoryProviders>
      <VoiceDegradedNotice degraded={false} />
      <StaleThreadNotice stale={false} />
    </StoryProviders>
  ),
};
