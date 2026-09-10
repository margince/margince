// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuthOutcomeNote } from "./connectors.notices";
import { StoryProviders } from "./story-utils";

// What comes back from a consent round trip, which is the half of the
// connections card no fixture can pose: the note reads the ADDRESS the provider
// returned to, so each frame sets that address and mounts nothing else.
//
// `ok` and the five failures share one heading each — connected, or nothing
// was connected — because that is what a reader scans for; the outcome's own
// sentence under it says which remedy this particular failure wants.

const meta: Meta<typeof OAuthOutcomeNote> = {
  title: "Settings/You/Connections/OAuth return",
  component: OAuthOutcomeNote,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof OAuthOutcomeNote>;

function landing(outcome: string) {
  return () => {
    globalThis.location.hash = `#/settings/connections/${outcome}`;
    return (
      <StoryProviders>
        <OAuthOutcomeNote />
      </StoryProviders>
    );
  };
}

/** It worked, and the mailbox is capturing. */
export const Connected: Story = { render: landing("ok") };

/** The reader said no. Nothing is wrong and nothing was connected. */
export const Declined: Story = { render: landing("denied") };

/** The provider's API was never enabled: an administrator's console, not a retry. */
export const Misconfigured: Story = { render: landing("misconfigured") };

/** This deployment's client credentials were refused — the app card, not a retry. */
export const BadClient: Story = { render: landing("bad_client") };

/** The same refusal in dark, where tone reaches the heading's ink. */
export const BadClientDark: Story = {
  globals: { theme: "dark" },
  render: landing("bad_client"),
};

/**
 * An address carrying no outcome draws nothing. The segment is server-defined,
 * so an unknown one is ignored rather than printed back at the reader.
 */
export const NoOutcome: Story = { render: landing("constructor") };
